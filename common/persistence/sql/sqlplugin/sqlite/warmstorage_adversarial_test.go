//go:build !race

package sqlite

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pborman/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/uber/cadence/common"
	"github.com/uber/cadence/common/log/testlogger"
	"github.com/uber/cadence/common/metrics"
	"github.com/uber/cadence/common/persistence"
	pt "github.com/uber/cadence/common/persistence/persistence-tests"
	"github.com/uber/cadence/common/persistence/serialization"
	"github.com/uber/cadence/common/persistence/sql"
	"github.com/uber/cadence/common/persistence/wrappers/warmstorage"
	"github.com/uber/cadence/common/types"
)

type warmEnv struct {
	base     *pt.TestBase
	flag     *atomic.Bool
	domainID string
	we       types.WorkflowExecution
}

func newWarmEnv(t *testing.T) *warmEnv {
	flag := &atomic.Bool{}
	flag.Store(true)
	dc := persistence.NewDefaultDynamicConfiguration()
	dc.EnableWarmStorage = func(string) bool { return flag.Load() }
	base := pt.NewTestBase(t, pt.TestBaseParams{PersistenceConfig: pt.SimplePersistenceConfig(t, GetTestConfig), DynamicConfiguration: dc})
	base.Setup()
	return &warmEnv{base: base, flag: flag, domainID: uuid.New(), we: types.WorkflowExecution{WorkflowID: "wf-" + uuid.New(), RunID: uuid.New()}}
}

func (e *warmEnv) create(t *testing.T) {
	_, err := e.base.CreateWorkflowExecutionWithBranchToken(context.Background(), e.domainID, e.we, "tl", "wt", 100, 10, nil, 3, 0, 2, []byte("branch"), nil, nil)
	require.NoError(t, err)
}

func (e *warmEnv) req() *persistence.DeleteWorkflowExecutionRequest {
	return &persistence.DeleteWorkflowExecutionRequest{ShardID: common.IntPtr(e.base.ShardInfo.ShardID), DomainID: e.domainID, WorkflowID: e.we.WorkflowID, RunID: e.we.RunID}
}

func (e *warmEnv) getReq() *persistence.GetWorkflowExecutionRequest {
	return &persistence.GetWorkflowExecutionRequest{ShardID: common.IntPtr(e.base.ShardInfo.ShardID), DomainID: e.domainID, Execution: e.we}
}

// closeRun completes the run and fills in as many info fields as it can.
func (e *warmEnv) closeRun(t *testing.T) {
	ctx := context.Background()
	st, err := e.base.GetWorkflowExecutionInfo(ctx, e.domainID, e.we)
	require.NoError(t, err)
	info := st.ExecutionInfo
	info.State = persistence.WorkflowStateCompleted
	info.CloseStatus = persistence.WorkflowCloseStatusFailed
	info.CompletionEventBatchID = 2
	info.CompletionEvent = &types.HistoryEvent{ID: 2, EventType: types.EventTypeWorkflowExecutionFailed.Ptr()}
	info.Memo = map[string][]byte{"m": []byte("v")}
	info.SearchAttributes = map[string][]byte{"CustomKeywordField": []byte(`"x"`)}
	info.StickyTaskList = "sticky"
	info.StickyScheduleToStartTimeout = 7
	info.DecisionAttempt = 3
	info.DecisionVersion = 5
	info.CancelRequested = true
	info.CancelRequestID = uuid.New()
	info.SignalCount = 4
	info.ClientLibraryVersion = "1.2.3"
	info.ClientFeatureVersion = "f"
	info.ClientImpl = "go"
	info.LastEventTaskID = 99
	info.TaskListKind = types.TaskListKindSticky
	info.ScheduledExecutionTimestamp = 12345
	info.Attempt = 2
	info.HasRetryPolicy = true
	info.InitialInterval = 1
	info.BackoffCoefficient = 2
	info.MaximumInterval = 10
	info.MaximumAttempts = 5
	info.NonRetriableErrors = []string{"x"}
	info.ExpirationSeconds = 77
	info.CronSchedule = "* * * * *"
	info.IsCron = true
	info.CronOverlapPolicy = types.CronOverlapPolicyBufferOne
	info.PartitionConfig = map[string]string{"a": "b"}
	info.ExpirationTime = time.Now().Add(time.Hour).Truncate(time.Millisecond)
	info.ExecutionStatus = types.WorkflowExecutionStatusFailed
	info.AutoResetPoints = &types.ResetPoints{Points: []*types.ResetPointInfo{{BinaryChecksum: "bc", RunID: e.we.RunID, FirstDecisionCompletedID: 4}}}
	info.ActiveClusterSelectionPolicy = &types.ActiveClusterSelectionPolicy{ClusterAttribute: &types.ClusterAttribute{Scope: "s", Name: "n"}}
	require.NoError(t, e.base.UpdateWorkflowExecutionAndFinish(ctx, info, &persistence.ExecutionStats{HistorySize: 4321}, 3, st.VersionHistories))
}

// TestAdversarial_WarmReadEqualsHotRead: after the move the manager-level state must equal the hot state
// except for the documented drops (auto-reset points, checksum, pending maps).
func TestAdversarial_WarmReadEqualsHotRead(t *testing.T) {
	e := newWarmEnv(t)
	ctx := context.Background()
	e.create(t)
	e.closeRun(t)
	hot, err := e.base.GetWorkflowExecutionInfo(ctx, e.domainID, e.we)
	require.NoError(t, err)

	require.NoError(t, e.base.ExecutionManager.MoveToClosedExecutions(ctx, e.req()))
	warm, err := e.base.GetWorkflowExecutionInfo(ctx, e.domainID, e.we)
	require.NoError(t, err, "warm read")

	hot.ExecutionInfo.AutoResetPoints = &types.ResetPoints{} // the manager decodes a missing blob to an empty, non-nil value
	hot.ExecutionInfo.AutoResetPointsDropped = true
	assert.Equal(t, hot.ExecutionInfo, warm.ExecutionInfo)
	assert.Equal(t, hot.ExecutionStats, warm.ExecutionStats)
	assert.Equal(t, hot.VersionHistories, warm.VersionHistories)
}

// TestAdversarial_MoveConcurrently: several hosts moving the same run at once must all succeed.
func TestAdversarial_MoveConcurrently(t *testing.T) {
	e := newWarmEnv(t)
	ctx := context.Background()
	e.create(t)
	e.closeRun(t)
	var wg sync.WaitGroup
	errs := make([]error, 8)
	for i := range errs {
		wg.Add(1)
		go func(i int) { defer wg.Done(); errs[i] = e.base.ExecutionManager.MoveToClosedExecutions(ctx, e.req()) }(i)
	}
	wg.Wait()
	for _, err := range errs {
		assert.NoError(t, err)
	}
	_, err := e.base.GetWorkflowExecutionInfo(ctx, e.domainID, e.we)
	assert.NoError(t, err)
}

// TestAdversarial_WriteToMovedRun (VERIFICATION row 11, A8): an update or a conflict resolution that targets a
// moved run puts the row back and lands, so no event or task is lost (it used to fail with a missing row).
func TestAdversarial_WriteToMovedRun(t *testing.T) {
	writes := map[string]func(e *warmEnv, st *persistence.WorkflowMutableState) error{
		"update": func(e *warmEnv, st *persistence.WorkflowMutableState) error {
			info := st.ExecutionInfo
			condition := info.NextEventID
			info.NextEventID++
			return e.base.UpdateWorkflowExecution(context.Background(), info, st.ExecutionStats, st.VersionHistories, nil, nil, condition, nil, nil, nil, nil, nil)
		},
		"conflict resolve": func(e *warmEnv, st *persistence.WorkflowMutableState) error {
			info := st.ExecutionInfo
			condition := info.NextEventID
			info.NextEventID++
			return e.base.ConflictResolveWorkflowExecution(context.Background(), info, st.ExecutionStats, condition, nil, nil, nil, nil, nil, nil, st.VersionHistories)
		},
	}
	for name, write := range writes {
		t.Run(name, func(t *testing.T) {
			e := newWarmEnv(t)
			ctx := context.Background()
			e.create(t)
			e.closeRun(t)
			st, err := e.base.GetWorkflowExecutionInfo(ctx, e.domainID, e.we)
			require.NoError(t, err)
			next := st.ExecutionInfo.NextEventID
			require.NoError(t, e.base.ExecutionManager.MoveToClosedExecutions(ctx, e.req()))

			st.ExecutionInfo.AutoResetPoints = &types.ResetPoints{} // what a state loaded from the slim row writes back
			require.NoError(t, write(e, st))

			got, err := e.base.GetWorkflowExecutionInfo(ctx, e.domainID, e.we)
			require.NoError(t, err)
			assert.Equal(t, next+1, got.ExecutionInfo.NextEventID, "the write is not lost")
			assert.True(t, got.ExecutionInfo.AutoResetPointsDropped, "the restored run has no reset points and its slim row, so Describe rebuilds them")
		})
	}
}

// freshManager builds a new decorated manager over the same DB, i.e. a restarted process.
func (e *warmEnv) freshManager(t *testing.T) persistence.ExecutionManager {
	cfg := e.base.PersistenceConfig
	parser, err := serialization.NewParser(&e.base.DynamicConfiguration)
	require.NoError(t, err)
	f := sql.NewFactory(*cfg.DataStores[cfg.DefaultStore].SQL, "", e.base.Logger, parser, &e.base.DynamicConfiguration)
	exec, err := f.NewExecutionStore()
	require.NoError(t, err)
	closed, err := f.NewClosedExecutionStore()
	require.NoError(t, err)
	store := warmstorage.NewExecutionStore(exec, closed, parser, &e.base.DynamicConfiguration, testlogger.New(t), metrics.NewNoopMetricsClient())
	return persistence.NewExecutionManagerImpl(store, testlogger.New(t), persistence.NewPayloadSerializer(), &e.base.DynamicConfiguration)
}

// TestAdversarial_FlagOffAfterMoveKeepsReadsAndDeletes: on -> move -> off -> restarted process still reads and deletes.
func TestAdversarial_FlagOffAfterMoveKeepsReadsAndDeletes(t *testing.T) {
	e := newWarmEnv(t)
	ctx := context.Background()
	e.create(t)
	e.closeRun(t)
	require.NoError(t, e.base.ExecutionManager.MoveToClosedExecutions(ctx, e.req()))
	e.flag.Store(false)
	mgr := e.freshManager(t) // restarted process, flag off
	_, err := mgr.GetWorkflowExecution(ctx, e.getReq())
	require.NoError(t, err)
	require.NoError(t, mgr.DeleteWorkflowExecution(ctx, e.req()))
	_, err = mgr.GetWorkflowExecution(ctx, e.getReq())
	assert.IsType(t, &types.EntityNotExistsError{}, err)
}

// onInsertClosed runs a hook right before the slim row is written, i.e. after the move has read the executions row.
type onInsertClosed struct {
	persistence.ClosedExecutionStore
	hook func()
}

func (c *onInsertClosed) InsertClosedExecution(ctx context.Context, key persistence.ClosedExecutionKey, data *persistence.DataBlob) error {
	if key.WorkflowID != "cadence-warm-storage-sentinel" {
		c.hook()
	}
	return c.ClosedExecutionStore.InsertClosedExecution(ctx, key, data)
}

// TestAdversarial_UpdateBetweenReadAndDeleteIsNotLost (VERIFICATION row 30, A7): a writer that lands between the
// move's read of the executions row and its delete must keep its update. The delete is conditional on the row it
// copied, so it fails, the stale slim row is dropped, and the updated executions row stays the truth.
func TestAdversarial_UpdateBetweenReadAndDeleteIsNotLost(t *testing.T) {
	e := newWarmEnv(t)
	ctx := context.Background()
	e.create(t)
	e.closeRun(t)
	st, err := e.base.GetWorkflowExecutionInfo(ctx, e.domainID, e.we)
	require.NoError(t, err)
	info := st.ExecutionInfo
	condition := info.NextEventID

	cfg := e.base.PersistenceConfig
	parser, err := serialization.NewParser(&e.base.DynamicConfiguration)
	require.NoError(t, err)
	f := sql.NewFactory(*cfg.DataStores[cfg.DefaultStore].SQL, "", e.base.Logger, parser, &e.base.DynamicConfiguration)
	exec, err := f.NewExecutionStore()
	require.NoError(t, err)
	closed, err := f.NewClosedExecutionStore()
	require.NoError(t, err)
	hooked := &onInsertClosed{ClosedExecutionStore: closed, hook: func() {
		info.NextEventID = condition + 1
		require.NoError(t, e.base.UpdateWorkflowExecution(ctx, info, st.ExecutionStats, st.VersionHistories, nil, nil, condition, nil, nil, nil, nil, nil))
	}}
	store := warmstorage.NewExecutionStore(exec, hooked, parser, &e.base.DynamicConfiguration, testlogger.New(t), metrics.NewNoopMetricsClient())
	mgr := persistence.NewExecutionManagerImpl(store, testlogger.New(t), persistence.NewPayloadSerializer(), &e.base.DynamicConfiguration)

	require.NoError(t, mgr.MoveToClosedExecutions(ctx, e.req()))

	got, err := mgr.GetWorkflowExecution(ctx, e.getReq())
	require.NoError(t, err)
	assert.Equal(t, condition+1, got.State.ExecutionInfo.NextEventID, "the update is still there")
	assert.False(t, got.State.ExecutionInfo.AutoResetPointsDropped, "and it is read from executions, not from a stale slim row")
	_, err = closed.GetClosedExecution(ctx, persistence.ClosedExecutionKey{DomainID: e.domainID, WorkflowID: e.we.WorkflowID, RunID: e.we.RunID})
	assert.IsType(t, &types.EntityNotExistsError{}, err, "the stale slim row is gone")
}
