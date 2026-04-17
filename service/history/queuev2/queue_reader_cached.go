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

//go:generate mockgen -package $GOPACKAGE -destination queue_reader_cached_mock.go github.com/uber/cadence/service/history/queuev2 CachedQueueReader

import (
	"context"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/uber/cadence/common"
	"github.com/uber/cadence/common/clock"
	"github.com/uber/cadence/common/dynamicconfig/dynamicproperties"
	"github.com/uber/cadence/common/log"
	"github.com/uber/cadence/common/log/tag"
	"github.com/uber/cadence/common/metrics"
	"github.com/uber/cadence/common/persistence"
	"github.com/uber/cadence/service/history/shard"
)

// cachedQueueReaderOptions is the dynamic configuration for the cached queue reader.
type cachedQueueReaderOptions struct {
	Mode                  dynamicproperties.StringPropertyFn
	MaxSize               dynamicproperties.IntPropertyFn
	MaxLookAheadWindow    dynamicproperties.DurationPropertyFn
	PrefetchTriggerWindow dynamicproperties.DurationPropertyFn
	PrefetchPageSize      dynamicproperties.IntPropertyFn
	WarmupGracePeriod     dynamicproperties.DurationPropertyFn
	EvictionSafeWindow    dynamicproperties.DurationPropertyFn
	// MinPrefetchInterval is the minimum time between consecutive prefetch attempts.
	// It prevents the prefetch loop from hammering the database when the cache resets
	// or gap detection fires repeatedly.
	MinPrefetchInterval dynamicproperties.DurationPropertyFn
	// TimeEvictionInterval controls how often the time-based eviction loop fires.
	TimeEvictionInterval dynamicproperties.DurationPropertyFn
}

// CachedQueueReader extends QueueReader with cache injection and lifecycle control.
type CachedQueueReader interface {
	QueueReader
	Inject(tasks []persistence.Task)
	UpdateReadLevel(readLevel persistence.HistoryTaskKey)
	Start()
	Stop()
}

type cachedQueueReader struct {
	status  int32 // DaemonStatusInitialized / Started / Stopped — access only via atomic
	base    QueueReader
	queue   InMemQueue
	options *cachedQueueReaderOptions
	clock   clock.TimeSource
	logger  log.Logger
	metrics metrics.Scope

	mu sync.RWMutex

	// inclusiveLowerBound is the inclusive start of the cached window. Tasks
	// before this key have been evicted and are no longer served from cache.
	// Invariant: inclusiveLowerBound <= exclusiveUpperBound.
	inclusiveLowerBound persistence.HistoryTaskKey

	// exclusiveUpperBound is the exclusive end of the prefetched window. Tasks with
	// key < exclusiveUpperBound are covered by the cache if they exist in the DB.
	// Invariant: inclusiveLowerBound <= exclusiveUpperBound.
	// Always update via updateExclusiveUpperBound to keep the prefetch loop in sync.
	exclusiveUpperBound persistence.HistoryTaskKey

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// prefetchCh signals the prefetchLoop to recompute its timer. Buffered(1) so
	// senders never block; duplicate signals are dropped, the loop reads current
	// state on each wake.
	prefetchCh chan struct{}

	// injectAllowedAfter is the time before which Inject calls are silently
	// dropped (warmup period). Set once in the constructor from
	// clock.Now() + WarmupGracePeriod; never written again, so no mutex needed.
	injectAllowedAfter time.Time
}

func newCachedQueueReader(
	base QueueReader,
	queue InMemQueue,
	shard shard.Context,
	metricsScope metrics.Scope,
) *cachedQueueReader {
	config := shard.GetConfig()
	return newCachedQueueReaderWithOptions(base, queue, &cachedQueueReaderOptions{
		Mode:                  config.TimerProcessorCachedQueueReaderMode,
		MaxSize:               config.TimerProcessorCacheMaxSize,
		MaxLookAheadWindow:    config.TimerProcessorCacheMaxLookAheadWindow,
		PrefetchTriggerWindow: config.TimerProcessorCachePrefetchTriggerWindow,
		PrefetchPageSize:      config.TimerTaskBatchSize,
		WarmupGracePeriod:     config.TimerProcessorCacheWarmupGracePeriod,
		EvictionSafeWindow:    config.TimerProcessorCacheEvictionSafeWindow,
		MinPrefetchInterval:   config.TimerProcessorCacheMinPrefetchInterval,
		TimeEvictionInterval:  config.TimerProcessorCacheTimeEvictionInterval,
	}, shard.GetTimeSource(), shard.GetLogger(), metricsScope)
}

func newCachedQueueReaderWithOptions(
	base QueueReader,
	queue InMemQueue,
	options *cachedQueueReaderOptions,
	clockSource clock.TimeSource,
	logger log.Logger,
	metricsScope metrics.Scope,
) *cachedQueueReader {
	ctx, cancel := context.WithCancel(context.Background())
	return &cachedQueueReader{
		status:              common.DaemonStatusInitialized,
		base:                base,
		queue:               queue,
		options:             options,
		clock:               clockSource,
		logger:              logger,
		metrics:             metricsScope,
		inclusiveLowerBound: persistence.MinimumHistoryTaskKey,
		exclusiveUpperBound: persistence.MinimumHistoryTaskKey,
		prefetchCh:          make(chan struct{}, 1),
		ctx:                 ctx,
		cancel:              cancel,
		injectAllowedAfter:  clockSource.Now().Add(options.WarmupGracePeriod()),
	}
}

// Start anchors the initial eviction window and launches the background loops.
func (q *cachedQueueReader) Start() {
	if !atomic.CompareAndSwapInt32(&q.status, common.DaemonStatusInitialized, common.DaemonStatusStarted) {
		return
	}
	// Anchor the lower bound now so the cache doesn't serve tasks from the
	// beginning of time before the first ack-level update arrives.
	q.UpdateReadLevel(persistence.MinimumHistoryTaskKey)
	q.wg.Add(1)
	go q.prefetchLoop()
	q.wg.Add(1)
	go q.timeEvictionLoop()
}

// Stop cancels background goroutines and waits for them to finish.
func (q *cachedQueueReader) Stop() {
	if !atomic.CompareAndSwapInt32(&q.status, common.DaemonStatusStarted, common.DaemonStatusStopped) {
		return
	}
	q.cancel()
	q.wg.Wait()
}

// prefetchLoop fetches tasks into the look-ahead window on a timer. It fires
// initially after the warmup grace period, then re-arms based on the result
// or when the upper bound changes via notifyPrefetch.
func (q *cachedQueueReader) prefetchLoop() {
	defer q.wg.Done()

	timer := q.clock.NewTimer(q.options.WarmupGracePeriod())
	defer timer.Stop()

	for {
		select {
		case <-q.ctx.Done():
			q.logger.Info("prefetch loop stopping")
			return
		case <-q.prefetchCh:
			// Upper bound changed externally, recompute delay and reset timer.
			if !timer.Stop() {
				select {
				case <-timer.Chan():
				default:
				}
			}
			timer.Reset(q.nextPrefetchDelay())
		case <-timer.Chan():
			if err := q.prefetch(); err != nil {
				q.logger.Warn("prefetch failed, retrying shortly", tag.Error(err))
				timer.Reset(q.options.MinPrefetchInterval())
			} else {
				timer.Reset(q.nextPrefetchDelay())
			}
		}
	}
}

// timeEvictionLoop advances the lower bound on a fixed timer, independent of
// the ack-level updates in UpdateReadLevel.
func (q *cachedQueueReader) timeEvictionLoop() {
	defer q.wg.Done()

	timer := q.clock.NewTimer(q.options.TimeEvictionInterval())
	defer timer.Stop()

	for {
		select {
		case <-q.ctx.Done():
			q.logger.Info("eviction loop stopping")
			return
		case <-timer.Chan():
			q.timeEvict()
			timer.Reset(q.options.TimeEvictionInterval())
		}
	}
}

// notifyPrefetch signals the prefetchLoop to recompute its timer. Non-blocking;
// drops the signal if one is already pending, the loop reads current state on wake.
func (q *cachedQueueReader) notifyPrefetch() {
	select {
	case q.prefetchCh <- struct{}{}:
	default:
	}
}

// updateExclusiveUpperBound sets the upper bound and wakes the prefetchLoop.
// Caller must hold q.mu.
func (q *cachedQueueReader) updateExclusiveUpperBound(key persistence.HistoryTaskKey, reason string) {
	q.logger.Debug("upper bound advancing",
		tag.Dynamic("prevUpperBound", q.exclusiveUpperBound),
		tag.Dynamic("newUpperBound", key),
		tag.Dynamic("reason", reason),
	)
	q.exclusiveUpperBound = key
	q.notifyPrefetch()
}

// nextPrefetchDelay returns how long to wait before the next prefetch. It
// computes the trigger window relative to exclusiveUpperBound, clamped to
// MinPrefetchInterval.
func (q *cachedQueueReader) nextPrefetchDelay() time.Duration {
	q.mu.RLock()
	defer q.mu.RUnlock()
	var delay time.Duration
	upper := q.exclusiveUpperBound
	if !upper.Equal(persistence.MinimumHistoryTaskKey) {
		triggerTime := upper.GetScheduledTime().Add(-q.options.PrefetchTriggerWindow())
		if d := triggerTime.Sub(q.clock.Now()); d > 0 {
			delay = d
		}
	}
	if min := q.options.MinPrefetchInterval(); delay < min {
		return min
	}
	return delay
}

// isEnabled returns true if the cache is fully enabled
func (q *cachedQueueReader) isEnabled() bool { return q.options.Mode() == "enabled" }

// isShadow returns true if the cache is in shadow mode (results compared against DB but not used for processing)
func (q *cachedQueueReader) isShadow() bool { return q.options.Mode() == "shadow" }

// isDisabled returns true for the "disabled" mode and for any unrecognised
// value
func (q *cachedQueueReader) isDisabled() bool {
	if q.options.Mode() == "disabled" {
		return true
	}
	if q.isEnabled() || q.isShadow() {
		return false
	}

	// Default to disabled for unrecognized modes to
	// avoid unintended consequences of a bad config value.
	return true
}

// isInWarmup reports whether the warmup grace period has not yet elapsed.
// injectAllowedAfter is set once in the constructor and never written again,
// so no lock is needed.
func (q *cachedQueueReader) isInWarmup() bool {
	return q.clock.Now().Before(q.injectAllowedAfter)
}

// prefetch fetches one page of tasks into the look-ahead window. Returns nil
// on success (including no-op cases); non-nil on any failure. The caller
// (prefetchLoop) schedules the next attempt.
func (q *cachedQueueReader) prefetch() error {
	if q.isDisabled() {
		q.logger.Debug("prefetch skipped, cache disabled")
		return nil
	}

	// Snapshot capacity and upper bound together under one lock. Two separate
	// reads would let a concurrent putTasks or UpdateReadLevel slip in between,
	// giving us a stale starting position for the fetch.
	q.mu.RLock()
	availableCacheSize := q.options.MaxSize() - q.queue.Len()
	upperBound := q.exclusiveUpperBound
	q.mu.RUnlock()

	// Skip when the cache is full. Inserting now would trigger RTrimBySize,
	// evicting the freshly loaded tasks to make room and wasting a round-trip.
	// The prefetchLoop retries after MinPrefetchInterval once the processor
	// has consumed some entries.
	if availableCacheSize <= 0 {
		q.logger.Debug("prefetch skipped, cache full")
		return nil // not an error — the loop will reschedule via nextPrefetchDelay
	}

	now := q.clock.Now()

	// Ceiling of the look-ahead window; tasks at or after this time aren't due yet.
	exclusiveMaxKey := persistence.NewHistoryTaskKey(now.Add(q.options.MaxLookAheadWindow()), 0)

	// Start from the existing upper bound so pages don't overlap. On the first
	// run (upperBound is MinimumHistoryTaskKey, nothing fetched yet), anchor to
	// now-EvictionSafeWindow; starting from absolute minimum would pull tasks
	// that timeEvict would drop immediately.
	inclusiveMinTaskKey := upperBound
	if inclusiveMinTaskKey.Equal(persistence.MinimumHistoryTaskKey) {
		inclusiveMinTaskKey = persistence.NewHistoryTaskKey(now.Add(-q.options.EvictionSafeWindow()), 0)
	}

	// Window is already covered; skip the DB round-trip.
	if !inclusiveMinTaskKey.Less(exclusiveMaxKey) {
		q.logger.Debug("prefetch skipped, window already covered",
			tag.Dynamic("inclusiveMinTaskKey", inclusiveMinTaskKey),
			tag.Dynamic("exclusiveMaxKey", exclusiveMaxKey),
		)
		return nil
	}

	// Cap the page to available space (so the insert won't spill into RTrimBySize)
	// and to the configured page size (to bound each round-trip).
	pageSize := availableCacheSize
	if q.options.PrefetchPageSize() < pageSize {
		pageSize = q.options.PrefetchPageSize()
	}

	resp, err := q.base.GetTask(q.ctx, &GetTaskRequest{
		Progress: &GetTaskProgress{
			Range: Range{
				InclusiveMinTaskKey: inclusiveMinTaskKey,
				ExclusiveMaxTaskKey: exclusiveMaxKey,
			},
			NextPageToken: nil,
			NextTaskKey:   inclusiveMinTaskKey,
		},
		Predicate: NewUniversalPredicate(),
		PageSize:  pageSize,
	})
	if err != nil {
		q.logger.Error("prefetch failed", tag.Error(err))
		return fmt.Errorf("base.GetTask failed: %w", err)
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	// Upper bound changed while we held the lock, so another goroutine reset
	// the cache. The fetched tasks may not be contiguous with the new window.
	// Clear everything and start fresh.
	if !q.exclusiveUpperBound.Equal(upperBound) {
		q.logger.Info("gap detected, clearing cache",
			tag.Dynamic("prevUpper", upperBound),
			tag.Dynamic("newUpper", q.exclusiveUpperBound),
		)
		q.queue.Clear()
		q.updateExclusiveUpperBound(persistence.MinimumHistoryTaskKey, "gap-detected-reset")
		q.inclusiveLowerBound = persistence.MinimumHistoryTaskKey
		return fmt.Errorf("gap detected: upper bound changed during fetch")
	}

	// If a trim occurred, putTasks already updated the upper bound correctly.
	// Re-advancing would create false coverage.
	if q.putTasks(resp.Tasks) {
		return nil
	}

	// No trim: advance to the appropriate target.
	// Partial page → we've seen the full range, advance to the ceiling.
	// Full page → DB has more, advance only to the next task key.
	target := exclusiveMaxKey
	if len(resp.Tasks) >= pageSize {
		target = resp.Progress.NextTaskKey
	}
	if q.exclusiveUpperBound.Less(target) {
		q.updateExclusiveUpperBound(target, "prefetch-advance")
	}
	q.logger.Debug("prefetch complete",
		tag.Dynamic("tasksFetched", len(resp.Tasks)),
		tag.Dynamic("newUpper", q.exclusiveUpperBound),
		tag.Dynamic("cacheSize", q.queue.Len()),
	)
	return nil
}

// isRangeCovered reports whether [inclusiveMin, exclusiveMax) falls fully
// within the cached window [inclusiveLowerBound, exclusiveUpperBound).
// Caller must hold q.mu (read or write).
func (q *cachedQueueReader) isRangeCovered(inclusiveMin, exclusiveMax persistence.HistoryTaskKey) bool {
	return !inclusiveMin.Less(q.inclusiveLowerBound) && !exclusiveMax.Greater(q.exclusiveUpperBound)
}

// isTaskCovered reports whether the given task key falls within the cached window.
// Caller must hold q.mu (read or write).
func (q *cachedQueueReader) isTaskCovered(key persistence.HistoryTaskKey) bool {
	return !key.Less(q.inclusiveLowerBound) && key.Less(q.exclusiveUpperBound)
}

// putTasks adds tasks to the cache and enforces the size cap.
// Caller must hold q.mu.
// putTasks adds tasks to the cache and enforces the size cap.
// Returns true if RTrimBySize fired and updated exclusiveUpperBound,
// meaning the caller must not re-advance the bound.
// Caller must hold q.mu.
func (q *cachedQueueReader) putTasks(tasks []persistence.Task) bool {
	q.queue.PutTasks(tasks)
	newUpper, trimmed := q.queue.RTrimBySize(q.options.MaxSize())
	q.metrics.RecordHistogramValue(metrics.CachedQueueSizeHistogram, float64(q.queue.Len()))

	if !trimmed {
		return false
	}

	if !newUpper.Greater(persistence.MinimumHistoryTaskKey) {
		// RTrimBySize emptied the cache (MaxSize <= 0). Reset the upper bound to
		// avoid claiming coverage over a range for which the cache holds no tasks.
		q.updateExclusiveUpperBound(persistence.MinimumHistoryTaskKey, "rtrim-empty")
	} else {
		q.updateExclusiveUpperBound(newUpper, "rtrim-shrink")
	}
	return true
}

// updateInclusiveLowerBound advances inclusiveLowerBound to newKey if it's
// ahead, trimming evicted tasks. Caps at exclusiveUpperBound when set to
// preserve the lower <= upper invariant.
// Caller must hold q.mu (write).
func (q *cachedQueueReader) updateInclusiveLowerBound(newKey persistence.HistoryTaskKey, reason string) {
	if !q.exclusiveUpperBound.Equal(persistence.MinimumHistoryTaskKey) &&
		newKey.Greater(q.exclusiveUpperBound) {
		newKey = q.exclusiveUpperBound
	}

	if !newKey.Greater(q.inclusiveLowerBound) {
		return
	}

	q.logger.Debug("lower bound advancing",
		tag.Dynamic("prevLowerBound", q.inclusiveLowerBound),
		tag.Dynamic("newLowerBound", newKey),
		tag.Dynamic("exclusiveUpperBound", q.exclusiveUpperBound),
		tag.Dynamic("reason", reason),
	)

	q.inclusiveLowerBound = newKey
	q.queue.LTrim(newKey)
	q.metrics.RecordHistogramValue(metrics.CachedQueueSizeHistogram, float64(q.queue.Len()))
}

// timeEvict advances inclusiveLowerBound to now - EvictionSafeWindow, evicting
// tasks that are old enough to be safe to drop.
func (q *cachedQueueReader) timeEvict() {
	evictBefore := persistence.NewHistoryTaskKey(
		q.clock.Now().Add(-q.options.EvictionSafeWindow()), 0,
	)
	q.mu.Lock()
	defer q.mu.Unlock()

	q.updateInclusiveLowerBound(evictBefore, "time-eviction")
}

// UpdateReadLevel advances the lower bound to the processor's ack position.
// MaximumHistoryTaskKey means "no valid read level" and is treated as minimum.
func (q *cachedQueueReader) UpdateReadLevel(readLevel persistence.HistoryTaskKey) {
	q.mu.Lock()
	defer q.mu.Unlock()

	// MaximumHistoryTaskKey means "no valid read level"; treat as minimum.
	if readLevel.Equal(persistence.MaximumHistoryTaskKey) {
		readLevel = persistence.MinimumHistoryTaskKey
	}

	q.updateInclusiveLowerBound(readLevel, "read-level-update")
}

// Inject adds tasks within [inclusiveLowerBound, exclusiveUpperBound) to the
// in-memory queue. Tasks outside the window are skipped; the prefetch loop
// loads them as the window advances. No-op when the cache is off or during
// the warmup period.
func (q *cachedQueueReader) Inject(tasks []persistence.Task) {
	if q.isDisabled() {
		q.logger.Debug("inject skipped, cache disabled")
		return
	}

	// Skip during warmup period or if Start has not been called yet.
	if q.isInWarmup() {
		q.logger.Debug("inject skipped, not started or in warmup")
		return
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	logTags := []tag.Tag{
		tag.Dynamic("inclusiveLowerBound", q.inclusiveLowerBound),
		tag.Dynamic("exclusiveUpperBound", q.exclusiveUpperBound),
	}

	var covered []persistence.Task
	for _, t := range tasks {
		if q.isTaskCovered(t.GetTaskKey()) {
			covered = append(covered, t)
		}
	}

	if len(covered) == 0 {
		q.logger.Debug("no tasks within cache window", logTags...)
		return
	}

	q.putTasks(covered)
}

// GetTask serves tasks from the cache when the range is fully covered.
// Shadow mode always hits the DB and compares with the cache result to detect
// divergence. Disabled mode bypasses the cache entirely.
func (q *cachedQueueReader) GetTask(ctx context.Context, req *GetTaskRequest) (*GetTaskResponse, error) {
	if q.isDisabled() || q.isInWarmup() {
		q.logger.Debug("fail back to original get task, cache is disabled or in warmup")
		return q.base.GetTask(ctx, req)
	}

	q.mu.RLock()
	inclusiveLowerBound := q.inclusiveLowerBound
	logTags := []tag.Tag{
		tag.Dynamic("requestedRange", req.Progress.Range),
		tag.Dynamic("inclusiveLowerBound", q.inclusiveLowerBound),
		tag.Dynamic("exclusiveUpperBound", q.exclusiveUpperBound),
		tag.Dynamic("cacheSize", q.queue.Len()),
	}

	covered := q.isRangeCovered(req.Progress.NextTaskKey, req.Progress.ExclusiveMaxTaskKey)
	if !covered {
		q.mu.RUnlock()
		q.metrics.IncCounter(metrics.CachedQueueMissesCounter)
		q.logger.Debug("cache miss", logTags...)
		return q.base.GetTask(ctx, req)
	}

	cacheResp := q.queue.GetTasks(req)
	q.mu.RUnlock()

	q.metrics.IncCounter(metrics.CachedQueueHitsCounter)
	q.logger.Debug("cache hit", logTags...)

	if q.isShadow() {
		return q.getTaskInShadow(ctx, req, cacheResp, inclusiveLowerBound, logTags)
	}

	return cacheResp, nil
}

// getTaskInShadow always queries the DB and compares the result against the
// cache snapshot. Returns the DB result; mismatches are logged but don't
// affect processing.
func (q *cachedQueueReader) getTaskInShadow(
	ctx context.Context,
	req *GetTaskRequest,
	cacheResp *GetTaskResponse,
	snapshotLowerBound persistence.HistoryTaskKey,
	logTags []tag.Tag,
) (*GetTaskResponse, error) {
	dbResp, err := q.base.GetTask(ctx, req)
	if err != nil {
		q.logger.Error("shadow comparison skipped, base returned error",
			append(logTags, tag.Error(err))...,
		)
		return dbResp, err
	}

	// Re-read the current cache to filter tasks that arrived via Inject after
	// the snapshot (benign race) and tasks evicted below snapshotLowerBound.
	q.mu.RLock()
	liveResp := q.queue.GetTasks(&GetTaskRequest{
		Progress:  req.Progress,
		Predicate: req.Predicate,
		PageSize:  math.MaxInt32,
	})
	q.mu.RUnlock()

	result := findMismatchesInShadow(cacheResp, dbResp, snapshotLowerBound, liveResp)
	q.reportShadowComparison(result, cacheResp, dbResp, logTags)
	return dbResp, nil
}

// findMismatchesInShadowResult holds the outcome of a shadow comparison.
type findMismatchesInShadowResult struct {
	// MissingFromCache contains task keys present in the DB response but absent
	// from the cache snapshot (after filtering benign eviction and inject races).
	MissingFromCache []persistence.HistoryTaskKey
	// ExtraInCache contains task keys present in the cache snapshot but absent
	// from the DB response. These are typically benign (Inject races, tasks
	// written to cache before the DB read completes) but are still reported as
	// mismatches for observability via shadowMismatch.extraInCache in the log.
	ExtraInCache []persistence.HistoryTaskKey
	// NextKeyMismatch is true when the cache and DB disagree on the next-page
	// boundary key, meaning a subsequent GetTask would start at different points.
	NextKeyMismatch bool
	// HasMismatches is true when any of the above fields indicate divergence.
	HasMismatches bool
}

// reportShadowComparison logs the result of a shadow comparison and increments
// the mismatch metric when HasMismatches is true.
func (q *cachedQueueReader) reportShadowComparison(
	result findMismatchesInShadowResult,
	cacheResp *GetTaskResponse,
	dbResp *GetTaskResponse,
	logTags []tag.Tag,
) {
	comparisonTags := append(logTags,
		tag.Dynamic("dbTaskCount", len(dbResp.Tasks)),
		tag.Dynamic("cacheTaskCount", len(cacheResp.Tasks)),
	)
	if !result.HasMismatches {
		q.logger.Debug("shadow comparison matched", comparisonTags...)
		return
	}

	q.metrics.IncCounter(metrics.CachedQueueMismatchCounter)
	mismatchTags := append(comparisonTags,
		tag.Dynamic("shadowMismatch.missingFromCache", result.MissingFromCache),
		tag.Dynamic("shadowMismatch.extraInCache", result.ExtraInCache),
		tag.Dynamic("shadowMismatch.nextKeyMismatch", result.NextKeyMismatch),
	)
	if cacheResp.Progress != nil {
		mismatchTags = append(mismatchTags, tag.Dynamic("shadowMismatch.cacheNextKey", cacheResp.Progress.NextTaskKey))
	}
	if dbResp.Progress != nil {
		mismatchTags = append(mismatchTags, tag.Dynamic("shadowMismatch.dbNextKey", dbResp.Progress.NextTaskKey))
	}
	q.logger.Warn("shadow comparison mismatch", mismatchTags...)
}

// findMismatchesInShadow compares a cache snapshot response against the DB
// response for the same request and returns a findMismatchesInShadowResult.
//
// Task comparison filters two benign cases:
//   - keys below preFetchLowerBound: evicted between snapshot and live re-read
//   - for MissingFromCache: keys in liveResp arrived via Inject after the
//     snapshot (may not yet be visible to the DB read)
//
// Compares by taskID rather than full key: injected tasks have nanosecond
// timestamps whereas Cassandra stores millisecond precision, so full-key
// comparison produces false positives for every injected task.
func findMismatchesInShadow(
	snapshotResp *GetTaskResponse, // cache response captured before the DB read
	dbResp *GetTaskResponse,
	preFetchLowerBound persistence.HistoryTaskKey, // lower bound at snapshot time
	liveResp *GetTaskResponse,                     // cache re-read after the DB fetch
) findMismatchesInShadowResult {
	snapshotIDs := make(map[int64]struct{}, len(snapshotResp.Tasks))
	for _, t := range snapshotResp.Tasks {
		snapshotIDs[t.GetTaskID()] = struct{}{}
	}
	dbIDs := make(map[int64]struct{}, len(dbResp.Tasks))
	for _, t := range dbResp.Tasks {
		dbIDs[t.GetTaskID()] = struct{}{}
	}
	liveIDs := make(map[int64]struct{}, len(liveResp.Tasks))
	for _, t := range liveResp.Tasks {
		liveIDs[t.GetTaskID()] = struct{}{}
	}

	var result findMismatchesInShadowResult

	// DB tasks missing from cache snapshot.
	for _, t := range dbResp.Tasks {
		if _, found := snapshotIDs[t.GetTaskID()]; found {
			continue // in snapshot
		}
		if t.GetTaskKey().Less(preFetchLowerBound) {
			continue // evicted between snapshot and live re-read
		}
		if _, found := liveIDs[t.GetTaskID()]; found {
			continue // arrived via Inject after snapshot (benign race)
		}
		result.MissingFromCache = append(result.MissingFromCache, t.GetTaskKey())
	}

	// Cache snapshot tasks missing from DB — tracked separately; extra tasks are
	// benign given task independence (Inject races, tasks written but not yet
	// committed when the DB read fires).
	for _, t := range snapshotResp.Tasks {
		if _, found := dbIDs[t.GetTaskID()]; found {
			continue // in DB result
		}
		if t.GetTaskKey().Less(preFetchLowerBound) {
			continue // evicted between snapshot and live re-read
		}
		result.ExtraInCache = append(result.ExtraInCache, t.GetTaskKey())
	}

	// Compare progress: if NextTaskKey differs the caller would start the next
	// page at a different position — a meaningful divergence even when task sets
	// match. Guard against nil Progress (defensive; both sides should always set
	// it, but mocks sometimes omit it).
	if snapshotResp.Progress != nil && dbResp.Progress != nil {
		result.NextKeyMismatch = !snapshotResp.Progress.NextTaskKey.Equal(dbResp.Progress.NextTaskKey)
	}

	// HasMismatches is true when any of the above fields indicate divergence,
	// including ExtraInCache which is benign but still a mismatch to be observed.
	result.HasMismatches = len(result.MissingFromCache) > 0 || len(result.ExtraInCache) > 0 || result.NextKeyMismatch
	return result
}

// LookAHead returns the next task at or after req.InclusiveMinTaskKey. Serves
// from cache when the request falls within the prefetched window. Bypasses
// cache when disabled or in shadow mode.
func (q *cachedQueueReader) LookAHead(ctx context.Context, req *LookAHeadRequest) (*LookAHeadResponse, error) {
	if q.isDisabled() || q.isShadow() || q.isInWarmup() {
		q.logger.Debug("fail back to original look-ahead, cache is disabled, shadow mode or in warmup")
		return q.base.LookAHead(ctx, req)
	}

	q.mu.RLock()
	exclusiveUpperBound := q.exclusiveUpperBound
	inclusiveLowerBound := q.inclusiveLowerBound

	// MinimumHistoryTaskKey is the sentinel meaning "not yet initialized"; the
	// cache window is valid only after the first prefetch sets a real upper bound.
	// Also require the request starts at or after the lower bound — time eviction
	// can advance it past the caller's min key, in which case the cache has
	// evicted those tasks and the DB must be consulted.
	// Upper bound is exclusive: req == upper means the cache has no tasks there,
	// so use strict Less rather than !Greater to avoid a false coverage claim.
	covered := exclusiveUpperBound.Greater(persistence.MinimumHistoryTaskKey) &&
		!req.InclusiveMinTaskKey.Less(inclusiveLowerBound) &&
		req.InclusiveMinTaskKey.Less(exclusiveUpperBound)

	var cacheTask persistence.Task
	if covered {
		cacheTask = q.queue.LookAHead(req.InclusiveMinTaskKey)
	}
	q.mu.RUnlock()

	if covered {
		q.logger.Debug("look-ahead cache hit",
			tag.Dynamic("inclusiveMinTaskKey", req.InclusiveMinTaskKey),
			tag.Dynamic("exclusiveUpperBound", exclusiveUpperBound),
			tag.Dynamic("taskFound", cacheTask != nil),
		)
		return &LookAHeadResponse{
			Task:             cacheTask,
			LookAheadMaxTime: exclusiveUpperBound.GetScheduledTime(),
		}, nil
	}

	q.logger.Debug("look-ahead cache miss",
		tag.Dynamic("inclusiveMinTaskKey", req.InclusiveMinTaskKey),
		tag.Dynamic("exclusiveUpperBound", exclusiveUpperBound),
	)
	return q.base.LookAHead(ctx, req)
}
