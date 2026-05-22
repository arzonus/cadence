// The MIT License (MIT)

// Copyright (c) 2017-2020 Uber Technologies Inc.

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package queuev2

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/uber/cadence/common/clock"
	"github.com/uber/cadence/common/dynamicconfig/dynamicproperties"
	"github.com/uber/cadence/common/log/testlogger"
	"github.com/uber/cadence/common/metrics"
	"github.com/uber/cadence/common/persistence"
)

// testPrefetchInterval is the MinPrefetchInterval used in lifecycle tests.
// With a mock clock this value only affects nextPrefetchDelay() calculations,
// never real wall time.
const testPrefetchInterval = 20 * time.Millisecond

// cachedQueueReaderLifecycleDeps holds the mocks injected into a running
// cachedQueueReader during lifecycle tests.
type cachedQueueReaderLifecycleDeps struct {
	mockBase *MockQueueReader
	clock    clock.MockedTimeSource
}

// setupMocksForCachedQueueReaderLifecycle creates a reader wired with a mock clock
// and a mock base reader. Callers must register expectations, call r.Start(), and
// defer r.Stop().
//
// Timing config: MinPrefetchInterval=testPrefetchInterval (20 ms) and
// PrefetchTriggerWindow=1h ensure nextPrefetchDelay() always clamps to
// MinPrefetchInterval, so advancing the mock clock by testPrefetchInterval
// reliably fires the next prefetch.
func setupMocksForCachedQueueReaderLifecycle(
	t *testing.T,
	ctrl *gomock.Controller,
	overrides ...func(*cachedQueueReaderOptions),
) (*cachedQueueReader, *cachedQueueReaderLifecycleDeps) {
	t.Helper()
	deps := &cachedQueueReaderLifecycleDeps{
		mockBase: NewMockQueueReader(ctrl),
		clock:    clock.NewMockedTimeSource(),
	}
	allOverrides := append([]func(*cachedQueueReaderOptions){
		func(o *cachedQueueReaderOptions) {
			o.MinPrefetchInterval = dynamicproperties.GetDurationPropertyFn(testPrefetchInterval)
			o.PrefetchTriggerWindow = dynamicproperties.GetDurationPropertyFn(time.Hour)
		},
	}, overrides...)
	r := newCachedQueueReaderWithOptions(
		deps.mockBase,
		newInMemQueue(),
		deps.clock,
		testlogger.New(t),
		metrics.NoopScope,
		testOptions(allOverrides...),
	)
	return r, deps
}

// expectOnePrefetch registers one base.GetTask expectation and returns a channel
// that receives a value once the prefetch goroutine calls base.GetTask.
// The channel is buffered(1) so the sender never blocks.
func expectOnePrefetch(base *MockQueueReader, resp *GetTaskResponse) <-chan struct{} {
	done := make(chan struct{}, 1)
	base.EXPECT().GetTask(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ *GetTaskRequest) (*GetTaskResponse, error) {
			done <- struct{}{}
			return resp, nil
		},
	)
	return done
}

// waitForPrefetch blocks until the prefetch goroutine signals done or 5 s elapse.
func waitForPrefetch(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for prefetch to complete")
	}
}

// triggerNextPrefetch waits for the prefetch goroutine to be idle (1 timer waiter),
// then advances the mock clock by d to fire its timer. After returning, the goroutine
// has woken but may not have finished the prefetch yet — call waitForPrefetch and
// then BlockUntil(1) to confirm completion and re-idling.
func triggerNextPrefetch(clk clock.MockedTimeSource, d time.Duration) {
	clk.BlockUntil(1)
	clk.Advance(d)
}

// readBounds reads both bounds under a read lock and returns them.
func readBounds(r *cachedQueueReader) (lower, upper persistence.HistoryTaskKey) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.inclusiveLowerBound, r.exclusiveUpperBound
}

// TestCachedQueueReader_Lifecycle_FirstPrefetchAnchorsWindow starts the reader and verifies
// that the first automatic prefetch anchors inclusiveLowerBound to clk.Now()-TimeEvictionWindow
// and sets exclusiveUpperBound to the NextTaskKey returned by the base reader.
func TestCachedQueueReader_Lifecycle_FirstPrefetchAnchorsWindow(t *testing.T) {
	ctrl := gomock.NewController(t)
	r, deps := setupMocksForCachedQueueReaderLifecycle(t, ctrl)
	base, clk := deps.mockBase, deps.clock

	// now is real time used only for task scheduled-time offsets (relative comparisons only).
	now := time.Now()
	nextKey := persistence.NewHistoryTaskKey(now.Add(20*time.Minute), 0)
	tasks := []persistence.Task{
		newTask(1, now.Add(1*time.Minute)),
		newTask(2, now.Add(5*time.Minute)),
		newTask(3, now.Add(8*time.Minute)),
		newTask(4, now.Add(12*time.Minute)),
		newTask(5, now.Add(15*time.Minute)),
	}

	// Capture the InclusiveMinTaskKey from the prefetch request and the mock clock
	// at that instant; both are used for exact lower-bound assertions.
	var capturedMinKey persistence.HistoryTaskKey
	var prefetchClockNow time.Time
	done := make(chan struct{}, 1)
	base.EXPECT().GetTask(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, req *GetTaskRequest) (*GetTaskResponse, error) {
			capturedMinKey = req.Progress.Range.InclusiveMinTaskKey
			prefetchClockNow = clk.Now()
			done <- struct{}{}
			return &GetTaskResponse{
				Tasks:    tasks,
				Progress: &GetTaskProgress{NextTaskKey: nextKey},
			}, nil
		},
	)

	r.Start()
	defer r.Stop()

	triggerNextPrefetch(clk, time.Millisecond)
	waitForPrefetch(t, done)
	clk.BlockUntil(1) // goroutine has reset its timer and is idle

	gotLower, gotUpper := readBounds(r)

	r.mu.RLock()
	queueLen := r.queue.Len()
	r.mu.RUnlock()

	assert.True(t, gotLower.Equal(capturedMinKey), "lower: got %v want %v", gotLower, capturedMinKey)
	assert.True(t, gotUpper.Equal(nextKey), "upper: got %v want %v", gotUpper, nextKey)
	// Lower bound must be exactly clk.Now()-TimeEvictionWindow at the moment prefetch ran.
	wantLower := prefetchClockNow.Add(-time.Minute)
	assert.True(t, capturedMinKey.GetScheduledTime().Equal(wantLower),
		"lower bound: got %v want clk.Now()-TimeEvictionWindow = %v", capturedMinKey.GetScheduledTime(), wantLower)
	assert.Equal(t, 5, queueLen, "queue must hold all 5 fetched tasks")
}

// TestCachedQueueReader_Lifecycle_InjectBeforeWindowEstablished verifies that Inject is a no-op
// when called before the first prefetch has established the cache window. The prefetch goroutine
// is blocked at the DB call, so bounds are still at Minimum when Inject runs.
func TestCachedQueueReader_Lifecycle_InjectBeforeWindowEstablished(t *testing.T) {
	ctrl := gomock.NewController(t)
	r, deps := setupMocksForCachedQueueReaderLifecycle(t, ctrl)
	base, clk := deps.mockBase, deps.clock

	now := time.Now()
	task := newTask(1, now.Add(10*time.Minute))

	// Block the first prefetch at the base call; release once the test has injected and asserted.
	entering := make(chan struct{})
	released := make(chan struct{})

	base.EXPECT().GetTask(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ *GetTaskRequest) (*GetTaskResponse, error) {
			close(entering)
			<-released
			return &GetTaskResponse{
				Progress: &GetTaskProgress{
					NextTaskKey: persistence.NewHistoryTaskKey(now.Add(time.Hour), 0),
				},
			}, nil
		},
	)

	r.Start()
	defer r.Stop()

	// Fire the initial 1ms timer to start the first prefetch.
	triggerNextPrefetch(clk, time.Millisecond)

	// Wait until the prefetch goroutine has called base.GetTask (bounds still at Minimum).
	select {
	case <-entering:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for prefetch to reach base.GetTask")
	}

	// Inject while the prefetch holds bounds at Minimum — isTaskCovered returns false.
	r.Inject([]persistence.Task{task})

	// Assert inject was a no-op: both bounds remain Minimum, queue empty.
	gotLower, gotUpper := readBounds(r)
	r.mu.RLock()
	queueLen := r.queue.Len()
	r.mu.RUnlock()

	assert.True(t, gotLower.Equal(persistence.MinimumHistoryTaskKey), "lower: got %v want Minimum", gotLower)
	assert.True(t, gotUpper.Equal(persistence.MinimumHistoryTaskKey), "upper: got %v want Minimum", gotUpper)
	assert.Equal(t, 0, queueLen, "queue must remain empty after filtered inject")

	// Unblock the prefetch goroutine so Stop() can drain cleanly.
	close(released)
}

// TestCachedQueueReader_Lifecycle_GetTaskHitMissHit exercises a multi-step GetTask lifecycle
// with the reader fully running: cache hit → UpdateReadLevel advances lower bound → cache miss
// → goroutine prefetches more → cache hit from extended window.
//
// Expectations are registered inline after each prefetch completes to avoid races with the
// background goroutine. The mock clock prevents any unintended timer firings between steps.
func TestCachedQueueReader_Lifecycle_GetTaskHitMissHit(t *testing.T) {
	ctrl := gomock.NewController(t)
	r, deps := setupMocksForCachedQueueReaderLifecycle(t, ctrl)
	base, clk := deps.mockBase, deps.clock

	now := time.Now()
	task1 := newTask(1, now.Add(5*time.Minute))
	task2 := newTask(2, now.Add(10*time.Minute))
	task3 := newTask(3, now.Add(20*time.Minute))
	task4 := newTask(4, now.Add(25*time.Minute))
	task5 := newTask(5, now.Add(50*time.Minute))

	windowUpper := newTimeKey(now.Add(time.Hour))

	task6 := newTask(6, now.Add(time.Hour))
	task7 := newTask(7, now.Add(time.Hour+10*time.Minute))
	task8 := newTask(8, now.Add(time.Hour+20*time.Minute))
	extendedUpper := newTimeKey(now.Add(time.Hour + 30*time.Minute))

	// Only the first prefetch expectation is pre-registered. The miss expectation and done2
	// are registered inline after done1 fires so the goroutine cannot claim them early.
	done1 := expectOnePrefetch(base, &GetTaskResponse{
		Tasks:    []persistence.Task{task1, task2, task3, task4, task5},
		Progress: &GetTaskProgress{NextTaskKey: windowUpper},
	})

	r.Start()
	defer r.Stop()

	// --- Step 1: first prefetch fills window ---
	triggerNextPrefetch(clk, time.Millisecond)
	waitForPrefetch(t, done1)
	clk.BlockUntil(1) // goroutine has reset its timer and is idle — safe to register expectations

	// Register miss (consumed by the test's GetTask below) and done2 (consumed by the goroutine).
	missResp := &GetTaskResponse{
		Tasks: nil,
		Progress: &GetTaskProgress{
			Range: Range{
				InclusiveMinTaskKey: newTimeKey(now.Add(15 * time.Minute)),
				ExclusiveMaxTaskKey: newTimeKey(now.Add(30 * time.Minute)),
			},
			NextTaskKey: newTimeKey(now.Add(30 * time.Minute)),
		},
	}
	base.EXPECT().GetTask(gomock.Any(), gomock.Any()).Return(missResp, nil).Times(1)

	done2 := make(chan struct{}, 1)
	base.EXPECT().GetTask(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ *GetTaskRequest) (*GetTaskResponse, error) {
			done2 <- struct{}{}
			return &GetTaskResponse{
				Tasks:    []persistence.Task{task6, task7, task8},
				Progress: &GetTaskProgress{NextTaskKey: extendedUpper},
			}, nil
		},
	)

	// --- Step 2: GetTask hit — range [now, now+1h) fully within window ---
	resp1, err := r.GetTask(context.Background(), &GetTaskRequest{
		Progress:  newProgress(newTimeKey(now), windowUpper),
		Predicate: NewUniversalPredicate(),
		PageSize:  10,
	})
	require.NoError(t, err)
	assert.Equal(t, []persistence.Task{task1, task2, task3, task4, task5}, resp1.Tasks,
		"step 2: cache hit must return all 5 tasks")

	// --- Step 3: UpdateReadLevel(now+20min) advances lower bound ---
	updateKey := newTimeKey(now.Add(20 * time.Minute))
	r.UpdateReadLevel(updateKey)

	gotLower, _ := readBounds(r)
	assert.True(t, gotLower.Equal(updateKey), "lower after UpdateReadLevel: got %v want %v", gotLower, updateKey)

	// --- Step 4: GetTask miss — start at now+15min is before new lower (now+20min) ---
	resp2, err := r.GetTask(context.Background(), &GetTaskRequest{
		Progress:  newProgress(newTimeKey(now.Add(15*time.Minute)), newTimeKey(now.Add(30*time.Minute))),
		Predicate: NewUniversalPredicate(),
		PageSize:  10,
	})
	require.NoError(t, err)
	assert.Equal(t, missResp, resp2, "step 4: cache miss must delegate to base")

	// --- Step 5: trigger goroutine → second prefetch extends window ---
	// UpdateReadLevel does not call notifyPrefetch, so we must advance the clock.
	triggerNextPrefetch(clk, testPrefetchInterval)
	waitForPrefetch(t, done2)
	clk.BlockUntil(1) // goroutine has reset its timer and is idle

	_, gotUpper := readBounds(r)
	assert.True(t, gotUpper.Equal(extendedUpper), "upper after second prefetch: got %v want %v", gotUpper, extendedUpper)

	// --- Step 6: GetTask hit from extended window ---
	resp3, err := r.GetTask(context.Background(), &GetTaskRequest{
		Progress:  newProgress(newTimeKey(now.Add(25*time.Minute)), newTimeKey(now.Add(time.Hour+25*time.Minute))),
		Predicate: NewUniversalPredicate(),
		PageSize:  10,
	})
	require.NoError(t, err)
	// task4(25min), task5(50min), task6(1h), task7(1h10min), task8(1h20min)
	assert.Equal(t, []persistence.Task{task4, task5, task6, task7, task8}, resp3.Tasks,
		"step 6: cache hit must return 5 tasks from extended window")
}

// TestCachedQueueReader_Lifecycle_GetTaskPaginationStaysInCache verifies that paginating through
// cached results never hits the DB after the initial prefetch. Both pages are served from the
// in-memory queue while the reader goroutine is running.
func TestCachedQueueReader_Lifecycle_GetTaskPaginationStaysInCache(t *testing.T) {
	ctrl := gomock.NewController(t)
	r, deps := setupMocksForCachedQueueReaderLifecycle(t, ctrl)
	base, clk := deps.mockBase, deps.clock

	now := time.Now()
	// Use 2h upper so task 20 (at now+60min) falls strictly inside the exclusive range.
	windowUpper := newTimeKey(now.Add(2 * time.Hour))

	tasks := make([]persistence.Task, 20)
	for i := 0; i < 20; i++ {
		tasks[i] = newTask(int64(i+1), now.Add(time.Duration(i+1)*3*time.Minute))
	}

	// base.GetTask called exactly once (for the prefetch); pages 1 and 2 serve from cache.
	done := expectOnePrefetch(base, &GetTaskResponse{
		Tasks:    tasks,
		Progress: &GetTaskProgress{NextTaskKey: windowUpper},
	})

	r.Start()
	defer r.Stop()

	triggerNextPrefetch(clk, time.Millisecond)
	waitForPrefetch(t, done)
	clk.BlockUntil(1) // goroutine has reset its timer and is idle

	r.mu.RLock()
	queueLen := r.queue.Len()
	r.mu.RUnlock()
	require.Equal(t, 20, queueLen, "prefetch must populate 20 tasks")

	// --- Page 1: no NextPageToken ---
	resp1, err := r.GetTask(context.Background(), &GetTaskRequest{
		Progress:  newProgress(newTimeKey(now), windowUpper),
		Predicate: NewUniversalPredicate(),
		PageSize:  10,
	})
	require.NoError(t, err)
	require.Len(t, resp1.Tasks, 10, "page 1 must return exactly 10 tasks")
	assert.Equal(t, tasks[:10], resp1.Tasks, "page 1 must match first 10")

	task11Key := resp1.Progress.NextTaskKey

	// --- Page 2: NextPageToken + NextTaskKey inside window ---
	resp2, err := r.GetTask(context.Background(), &GetTaskRequest{
		Progress: &GetTaskProgress{
			Range:         Range{InclusiveMinTaskKey: newTimeKey(now), ExclusiveMaxTaskKey: windowUpper},
			NextPageToken: []byte("tok"),
			NextTaskKey:   task11Key,
		},
		Predicate: NewUniversalPredicate(),
		PageSize:  10,
	})
	require.NoError(t, err)
	require.Len(t, resp2.Tasks, 10, "page 2 must return exactly 10 tasks")
	assert.Equal(t, tasks[10:], resp2.Tasks, "page 2 must match tasks 11-20")
	// gomock enforces Times(1) implicitly: no further base.GetTask call was made.
}

// TestCachedQueueReader_Lifecycle_LookAHeadTaskFoundThenConsumedThenNil validates the full
// LookAHead lifecycle: task found → consumed via UpdateReadLevel → next task found →
// all consumed → nil task with window max time still reported.
func TestCachedQueueReader_Lifecycle_LookAHeadTaskFoundThenConsumedThenNil(t *testing.T) {
	ctrl := gomock.NewController(t)
	r, deps := setupMocksForCachedQueueReaderLifecycle(t, ctrl)
	base, clk := deps.mockBase, deps.clock

	now := time.Now()
	windowUpper := newTimeKey(now.Add(time.Hour))

	task1 := newTask(1, now.Add(10*time.Minute))
	task2 := newTask(2, now.Add(20*time.Minute))
	task3 := newTask(3, now.Add(30*time.Minute))

	done := expectOnePrefetch(base, &GetTaskResponse{
		Tasks:    []persistence.Task{task1, task2, task3},
		Progress: &GetTaskProgress{NextTaskKey: windowUpper},
	})

	r.Start()
	defer r.Stop()

	triggerNextPrefetch(clk, time.Millisecond)
	waitForPrefetch(t, done)
	clk.BlockUntil(1) // goroutine has reset its timer and is idle

	_, gotUpper := readBounds(r)
	require.True(t, gotUpper.Equal(windowUpper), "upper: got %v want %v", gotUpper, windowUpper)

	// --- Step 2: LookAHead(now) → task1 ---
	resp1, err := r.LookAHead(context.Background(), &LookAHeadRequest{InclusiveMinTaskKey: newTimeKey(now)})
	require.NoError(t, err)
	assert.Equal(t, task1, resp1.Task, "LookAHead(now): expected task1")
	assert.Equal(t, windowUpper.GetScheduledTime(), resp1.LookAheadMaxTime,
		"LookAheadMaxTime must equal window upper throughout")

	// --- Step 3: consume task1 and task2 ---
	r.UpdateReadLevel(newTimeKey(now.Add(21 * time.Minute)))

	r.mu.RLock()
	gotLen := r.queue.Len()
	r.mu.RUnlock()
	assert.Equal(t, 1, gotLen, "queue must hold 1 task (task3)")

	// --- Step 4: LookAHead(now+21min) → task3 ---
	resp2, err := r.LookAHead(context.Background(), &LookAHeadRequest{InclusiveMinTaskKey: newTimeKey(now.Add(21 * time.Minute))})
	require.NoError(t, err)
	assert.Equal(t, task3, resp2.Task, "LookAHead(now+21min): expected task3")
	assert.Equal(t, windowUpper.GetScheduledTime(), resp2.LookAheadMaxTime,
		"LookAheadMaxTime must equal window upper throughout")

	// --- Step 5: consume task3 ---
	r.UpdateReadLevel(newTimeKey(now.Add(31 * time.Minute)))

	r.mu.RLock()
	gotLen = r.queue.Len()
	r.mu.RUnlock()
	assert.Equal(t, 0, gotLen, "queue must be empty")

	// --- Step 6: LookAHead(now+31min) → nil, window max still valid ---
	resp3, err := r.LookAHead(context.Background(), &LookAHeadRequest{InclusiveMinTaskKey: newTimeKey(now.Add(31 * time.Minute))})
	require.NoError(t, err)
	assert.Nil(t, resp3.Task, "LookAHead after all consumed: Task must be nil")
	assert.Equal(t, windowUpper.GetScheduledTime(), resp3.LookAheadMaxTime,
		"LookAheadMaxTime must equal window upper even with empty queue")
}

// TestCachedQueueReader_Lifecycle_InjectCausesCapEvictionAndWindowShrinks verifies that the
// running reader correctly handles Inject → RTrimBySize → window shrink → goroutine picks up
// the new (shrunken) upper bound as the start of its next prefetch.
func TestCachedQueueReader_Lifecycle_InjectCausesCapEvictionAndWindowShrinks(t *testing.T) {
	ctrl := gomock.NewController(t)
	r, deps := setupMocksForCachedQueueReaderLifecycle(t, ctrl, func(o *cachedQueueReaderOptions) {
		o.MaxSize = dynamicproperties.GetIntPropertyFn(5)
	})
	base, clk := deps.mockBase, deps.clock

	now := time.Now()
	t1 := newTask(1, now.Add(5*time.Minute))
	t2 := newTask(2, now.Add(15*time.Minute))
	t3 := newTask(3, now.Add(25*time.Minute))
	firstNextKey := newTimeKey(now.Add(time.Hour))

	t4 := newTask(4, now.Add(35*time.Minute))
	t5 := newTask(5, now.Add(45*time.Minute))
	t6 := newTask(6, now.Add(55*time.Minute))
	wantTrimKey := t5.GetTaskKey().Next()

	t7 := newTask(7, now.Add(56*time.Minute))
	t8 := newTask(8, now.Add(58*time.Minute))
	secondNextKey := newTimeKey(now.Add(2 * time.Hour))

	// First prefetch: fills window [now-1min, now+1h) with 3 tasks.
	done1 := expectOnePrefetch(base, &GetTaskResponse{
		Tasks:    []persistence.Task{t1, t2, t3},
		Progress: &GetTaskProgress{NextTaskKey: firstNextKey},
	})

	// Second prefetch: goroutine must start from wantTrimKey (the shrunken upper), not firstNextKey.
	var capturedSecondMinKey persistence.HistoryTaskKey
	done2 := make(chan struct{}, 1)
	base.EXPECT().GetTask(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, req *GetTaskRequest) (*GetTaskResponse, error) {
			capturedSecondMinKey = req.Progress.Range.InclusiveMinTaskKey
			done2 <- struct{}{}
			return &GetTaskResponse{
				Tasks:    []persistence.Task{t7, t8},
				Progress: &GetTaskProgress{NextTaskKey: secondNextKey},
			}, nil
		},
	)

	r.Start()
	defer r.Stop()

	// --- Step 1: first prefetch establishes window ---
	triggerNextPrefetch(clk, time.Millisecond)
	waitForPrefetch(t, done1)
	clk.BlockUntil(1) // goroutine has reset its timer and is idle

	r.mu.RLock()
	assert.Equal(t, 3, r.queue.Len(), "queue: 3 tasks after first prefetch")
	r.mu.RUnlock()

	// --- Step 2: inject 3 tasks — 3+3=6 > MaxSize(5) → RTrimBySize keeps 5, drops t6 ---
	// Inject calls notifyPrefetch via updateExclusiveUpperBound; goroutine resets timer.
	r.Inject([]persistence.Task{t4, t5, t6})

	r.mu.RLock()
	gotUpper := r.exclusiveUpperBound
	queueLen := r.queue.Len()
	r.mu.RUnlock()

	assert.Equal(t, 5, queueLen, "queue: 5 tasks after inject+trim")
	assert.True(t, gotUpper.Equal(wantTrimKey), "upper after inject+trim: got %v want trimKey %v", gotUpper, wantTrimKey)

	// --- Step 3: consume first 3 tasks to free capacity ---
	r.UpdateReadLevel(t3.GetTaskKey().Next())

	r.mu.RLock()
	queueLen = r.queue.Len()
	r.mu.RUnlock()
	assert.Equal(t, 2, queueLen, "queue: 2 tasks after UpdateReadLevel past t3")

	// --- Step 4: trigger goroutine → second prefetch must start from wantTrimKey ---
	// Inject already triggered notifyPrefetch; goroutine reset its timer then went back to select.
	triggerNextPrefetch(clk, testPrefetchInterval)
	waitForPrefetch(t, done2)
	clk.BlockUntil(1) // goroutine has reset its timer and is idle

	assert.True(t, capturedSecondMinKey.Equal(wantTrimKey),
		"second prefetch start key: got %v want trimKey %v", capturedSecondMinKey, wantTrimKey)

	_, gotUpper = readBounds(r)
	assert.True(t, gotUpper.Equal(secondNextKey), "upper after second prefetch: got %v want %v", gotUpper, secondNextKey)

	r.mu.RLock()
	queueLen = r.queue.Len()
	r.mu.RUnlock()
	assert.Equal(t, 4, queueLen, "queue: 2 surviving + 2 new tasks")
}

// TestCachedQueueReader_Lifecycle_TimeEvictionOnPutTasksOverflow verifies that the running reader
// performs lazy time eviction during Inject when inserting would exceed MaxSize.
//
// Design: MaxSize=10 so that the background tryTimeEvictIfCacheFull (which checks Len+1 >= MaxSize)
// does NOT fire with only 5 initial tasks (5+1=6 < 10). Time eviction fires when Inject(6 tasks)
// triggers tryTimeEvict(6): 5+6=11 >= 10, so tasks older than clk.Now()-TimeEvictionWindow are
// evicted. TimeEvictionWindow=30s; t1 is scheduled 40s before clk.Now() so it gets evicted.
//
// Tasks are created relative to clk.Now() captured inside the DoAndReturn, ensuring exact,
// reproducible eviction behavior with no real-time dependency.
func TestCachedQueueReader_Lifecycle_TimeEvictionOnPutTasksOverflow(t *testing.T) {
	ctrl := gomock.NewController(t)
	r, deps := setupMocksForCachedQueueReaderLifecycle(t, ctrl, func(o *cachedQueueReaderOptions) {
		o.MaxSize = dynamicproperties.GetIntPropertyFn(10)
		o.TimeEvictionWindow = dynamicproperties.GetDurationPropertyFn(30 * time.Second)
	})
	base, clk := deps.mockBase, deps.clock

	var (
		prefetchClockNow time.Time
		t1, t2, t3, t4, t5 persistence.Task
		firstNextKey        persistence.HistoryTaskKey
	)
	done := make(chan struct{}, 1)
	base.EXPECT().GetTask(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ *GetTaskRequest) (*GetTaskResponse, error) {
			// Capture mock clock time here; all task scheduled times are relative to it,
			// giving exact and deterministic eviction behaviour.
			prefetchClockNow = clk.Now()
			t1 = newTask(1, prefetchClockNow.Add(-40*time.Second)) // older than eviction horizon → evicted
			t2 = newTask(2, prefetchClockNow.Add(-10*time.Second)) // newer than eviction horizon → kept
			t3 = newTask(3, prefetchClockNow.Add(30*time.Second))
			t4 = newTask(4, prefetchClockNow.Add(60*time.Second))
			t5 = newTask(5, prefetchClockNow.Add(90*time.Second))
			firstNextKey = newTimeKey(prefetchClockNow.Add(3 * time.Minute))
			done <- struct{}{}
			return &GetTaskResponse{
				Tasks:    []persistence.Task{t1, t2, t3, t4, t5},
				Progress: &GetTaskProgress{NextTaskKey: firstNextKey},
			}, nil
		},
	)

	r.Start()
	defer r.Stop()

	triggerNextPrefetch(clk, time.Millisecond)
	waitForPrefetch(t, done)
	clk.BlockUntil(1) // goroutine has reset its timer and is idle

	r.mu.RLock()
	assert.Equal(t, 5, r.queue.Len(), "queue: 5 tasks after first prefetch")
	r.mu.RUnlock()

	// Advance the mock clock by 2ms so that clk.Now()-TimeEvictionWindow is strictly
	// greater than inclusiveLowerBound (which was set at clk.Now()-TimeEvictionWindow
	// during the prefetch). Without this, tryTimeEvict's advanceInclusiveLowerBound
	// would see evictBefore == inclusiveLowerBound and skip eviction (no-op).
	// 2ms is well below testPrefetchInterval (20ms), so the goroutine's timer won't fire.
	clk.Advance(2 * time.Millisecond)

	// Inject 6 tasks: 5+6=11 >= MaxSize(10) → tryTimeEvict fires.
	// evictBefore = clk.Now() - 30s = prefetchClockNow + 2ms - 30s > inclusiveLowerBound.
	// t1 (prefetchClockNow-40s) < evictBefore → evicted.
	// t2 (prefetchClockNow-10s) > evictBefore → kept. After eviction, 4+6=10 = MaxSize → no RTrimBySize.
	injectTasks := []persistence.Task{
		newTask(6, prefetchClockNow.Add(5*time.Second)),
		newTask(7, prefetchClockNow.Add(15*time.Second)),
		newTask(8, prefetchClockNow.Add(40*time.Second)),
		newTask(9, prefetchClockNow.Add(70*time.Second)),
		newTask(10, prefetchClockNow.Add(80*time.Second)),
		newTask(11, prefetchClockNow.Add(100*time.Second)),
	}
	r.Inject(injectTasks)

	r.mu.RLock()
	gotLower := r.inclusiveLowerBound
	gotUpper := r.exclusiveUpperBound
	queueLen := r.queue.Len()
	r.mu.RUnlock()

	// Lower must have advanced to exactly clk.Now()-TimeEvictionWindow.
	// clk.Now() at inject time = prefetchClockNow + 2ms (from the extra Advance above).
	wantEvictedLower := clk.Now().Add(-30 * time.Second)
	assert.True(t, gotLower.GetScheduledTime().Equal(wantEvictedLower),
		"lower bound: got %v want clk.Now()-TimeEvictionWindow = %v", gotLower.GetScheduledTime(), wantEvictedLower)
	// Upper unchanged — no cap trim (4 old tasks kept + 6 new = 10 = MaxSize).
	assert.True(t, gotUpper.Equal(firstNextKey), "upper: unchanged after time eviction: got %v want %v", gotUpper, firstNextKey)
	assert.Equal(t, 10, queueLen, "queue: 4 kept old tasks + 6 injected = 10 tasks")

	// Verify surviving tasks are accessible via cache GetTask.
	resp, err := r.GetTask(context.Background(), &GetTaskRequest{
		Progress:  newProgress(gotLower, firstNextKey),
		Predicate: NewUniversalPredicate(),
		PageSize:  20,
	})
	require.NoError(t, err)
	require.Len(t, resp.Tasks, 10, "GetTask: expected 10 tasks from cache")
	// t1 must not appear; t2 and all inject tasks must appear.
	for _, task := range resp.Tasks {
		assert.NotEqual(t, t1.GetTaskID(), task.GetTaskID(), "evicted t1 must not appear in cache results")
	}
	assert.Equal(t, t2.GetTaskID(), resp.Tasks[0].GetTaskID(), "first task in cache should be t2 (oldest surviving)")
}

// TestCachedQueueReader_Lifecycle_ClearAndPrefetchReAnchors verifies the clear-and-re-anchor
// lifecycle with the reader goroutine running throughout. After Clear, the goroutine automatically
// detects the Minimum bounds and re-anchors the window on the next prefetch cycle.
func TestCachedQueueReader_Lifecycle_ClearAndPrefetchReAnchors(t *testing.T) {
	ctrl := gomock.NewController(t)
	r, deps := setupMocksForCachedQueueReaderLifecycle(t, ctrl)
	base, clk := deps.mockBase, deps.clock

	now := time.Now()
	firstNextKey := persistence.NewHistoryTaskKey(now.Add(20*time.Minute), 0)
	secondNextKey := persistence.NewHistoryTaskKey(now.Add(15*time.Minute), 0)

	done1 := expectOnePrefetch(base, &GetTaskResponse{
		Tasks: []persistence.Task{
			newTask(1, now.Add(2*time.Minute)),
			newTask(2, now.Add(5*time.Minute)),
			newTask(3, now.Add(8*time.Minute)),
			newTask(4, now.Add(12*time.Minute)),
			newTask(5, now.Add(15*time.Minute)),
		},
		Progress: &GetTaskProgress{NextTaskKey: firstNextKey},
	})

	// Second prefetch after Clear: re-anchors lower to clk.Now()-TimeEvictionWindow.
	var capturedSecondMinKey persistence.HistoryTaskKey
	var secondPrefetchClockNow time.Time
	done2 := make(chan struct{}, 1)
	base.EXPECT().GetTask(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, req *GetTaskRequest) (*GetTaskResponse, error) {
			capturedSecondMinKey = req.Progress.Range.InclusiveMinTaskKey
			secondPrefetchClockNow = clk.Now()
			done2 <- struct{}{}
			return &GetTaskResponse{
				Tasks: []persistence.Task{
					newTask(6, now.Add(3*time.Minute)),
					newTask(7, now.Add(7*time.Minute)),
					newTask(8, now.Add(11*time.Minute)),
				},
				Progress: &GetTaskProgress{NextTaskKey: secondNextKey},
			}, nil
		},
	)

	r.Start()
	defer r.Stop()

	// --- Step 1: first prefetch establishes window ---
	triggerNextPrefetch(clk, time.Millisecond)
	waitForPrefetch(t, done1)
	clk.BlockUntil(1) // goroutine has reset its timer and is idle

	_, gotUpper := readBounds(r)
	assert.True(t, gotUpper.Equal(firstNextKey), "upper after first prefetch: got %v want %v", gotUpper, firstNextKey)

	r.mu.RLock()
	assert.Equal(t, 5, r.queue.Len(), "queue: 5 tasks after first prefetch")
	r.mu.RUnlock()

	// --- Step 2: Clear resets both bounds to Minimum and empties the queue ---
	// Clear calls notifyPrefetch; goroutine processes prefetchCh and resets timer.
	r.Clear()

	gotLower, gotUpper := readBounds(r)
	r.mu.RLock()
	queueLen := r.queue.Len()
	r.mu.RUnlock()

	assert.True(t, gotLower.Equal(persistence.MinimumHistoryTaskKey), "lower after Clear: got %v want Minimum", gotLower)
	assert.True(t, gotUpper.Equal(persistence.MinimumHistoryTaskKey), "upper after Clear: got %v want Minimum", gotUpper)
	assert.Equal(t, 0, queueLen, "queue: empty after Clear")

	// --- Step 3: goroutine detects Minimum bounds and re-anchors on next prefetch cycle ---
	triggerNextPrefetch(clk, testPrefetchInterval)
	waitForPrefetch(t, done2)
	clk.BlockUntil(1) // goroutine has reset its timer and is idle

	gotLower, gotUpper = readBounds(r)
	r.mu.RLock()
	queueLen = r.queue.Len()
	r.mu.RUnlock()

	// Lower must be re-anchored to exactly clk.Now()-TimeEvictionWindow at second prefetch time.
	wantSecondLower := secondPrefetchClockNow.Add(-time.Minute)
	assert.True(t, capturedSecondMinKey.GetScheduledTime().Equal(wantSecondLower),
		"second prefetch lower: got %v want clk.Now()-TimeEvictionWindow = %v",
		capturedSecondMinKey.GetScheduledTime(), wantSecondLower)
	assert.True(t, gotLower.Equal(capturedSecondMinKey), "lower after second prefetch: got %v want %v", gotLower, capturedSecondMinKey)
	assert.True(t, gotUpper.Equal(secondNextKey), "upper after second prefetch: got %v want %v", gotUpper, secondNextKey)
	assert.Equal(t, 3, queueLen, "queue: 3 tasks after second prefetch")
}
