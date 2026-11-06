package persistencetests

import (
	"testing"

	"github.com/pborman/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/uber/cadence/common"
	"github.com/uber/cadence/common/dynamicconfig/dynamicproperties"
	"github.com/uber/cadence/common/metrics"
	"github.com/uber/cadence/common/persistence"
	"github.com/uber/cadence/common/persistence/client"
	"github.com/uber/cadence/common/types"
)

// CheckWarmStorage moves a closed run through an ExecutionManager with warm storage on, against the real
// datastore of an already set up TestBase: move, read the slim row, delete both rows.
func CheckWarmStorage(t *testing.T, s *TestBase) {
	ctx := t.Context()
	dc := s.DynamicConfiguration
	dc.EnableWarmStorage = dynamicproperties.GetBoolPropertyFnFilteredByDomainID(true)
	cfg := s.PersistenceConfig
	mgr, err := client.NewFactory(&cfg, nil, s.ClusterMetadata.GetCurrentClusterName(), metrics.NewNoopMetricsClient(), s.Logger, &dc).NewExecutionManager()
	require.NoError(t, err)
	defer mgr.Close()

	domainID := uuid.New()
	execution := types.WorkflowExecution{WorkflowID: "warm-storage-wf", RunID: uuid.New()}
	_, err = s.CreateWorkflowExecution(ctx, domainID, execution, "tl", "wType", 20, 13, nil, 3, 0, 2, nil, nil)
	require.NoError(t, err)
	state, err := s.GetWorkflowExecutionInfo(ctx, domainID, execution)
	require.NoError(t, err)
	info := state.ExecutionInfo
	info.State = persistence.WorkflowStateCompleted
	info.CloseStatus = persistence.WorkflowCloseStatusCompleted
	// the slim row drops these, which tells a slim read from a hot one
	info.AutoResetPoints = &types.ResetPoints{Points: []*types.ResetPointInfo{{BinaryChecksum: "bc", RunID: execution.RunID, FirstDecisionCompletedID: 2}}}
	require.NoError(t, s.UpdateWorkflowExecutionAndFinish(ctx, info, state.ExecutionStats, info.NextEventID, state.VersionHistories))

	req := &persistence.DeleteWorkflowExecutionRequest{DomainID: domainID, WorkflowID: execution.WorkflowID, RunID: execution.RunID, ShardID: common.IntPtr(s.ShardInfo.ShardID)}
	require.NoError(t, mgr.MoveToClosedExecutions(ctx, req))

	getReq := &persistence.GetWorkflowExecutionRequest{DomainID: domainID, Execution: execution, ShardID: common.IntPtr(s.ShardInfo.ShardID), RangeID: s.ShardInfo.RangeID}
	resp, err := mgr.GetWorkflowExecution(ctx, getReq)
	require.NoError(t, err, "Warm run must be readable")
	got := resp.State.ExecutionInfo
	assert.True(t, got.AutoResetPointsDropped, "read came from the slim row")
	assert.Empty(t, got.AutoResetPoints.Points)
	assert.Equal(t, int64(3), got.NextEventID)
	assert.Equal(t, persistence.WorkflowStateCompleted, got.State)
	assert.Equal(t, persistence.WorkflowCloseStatusCompleted, got.CloseStatus)
	assert.Equal(t, "tl", got.TaskList)
	assert.Equal(t, "wType", got.WorkflowTypeName)
	assert.Equal(t, state.VersionHistories, resp.State.VersionHistories)

	exists, err := mgr.IsWorkflowExecutionExists(ctx, &persistence.IsWorkflowExecutionExistsRequest{DomainID: domainID, WorkflowID: execution.WorkflowID, RunID: execution.RunID, ShardID: common.IntPtr(s.ShardInfo.ShardID)})
	if err == nil { // SQL stores don't implement it
		assert.True(t, exists.Exists)
	}

	require.NoError(t, mgr.MoveToClosedExecutions(ctx, req), "second delivery of the close task is a no-op")
	require.NoError(t, mgr.DeleteWorkflowExecution(ctx, req))
	_, err = mgr.GetWorkflowExecution(ctx, getReq)
	assert.IsType(t, &types.EntityNotExistsError{}, err, "retention removes both rows")

	checkConditionalDelete(t, s, mgr)
}

// checkConditionalDelete covers the guard the move relies on: a delete with NextEventID set removes the executions
// row only if the row is still at that next event ID.
func checkConditionalDelete(t *testing.T, s *TestBase, mgr persistence.ExecutionManager) {
	ctx := t.Context()
	domainID := uuid.New()
	execution := types.WorkflowExecution{WorkflowID: "conditional-delete-wf", RunID: uuid.New()}
	_, err := s.CreateWorkflowExecution(ctx, domainID, execution, "tl", "wType", 20, 13, nil, 3, 0, 2, nil, nil)
	require.NoError(t, err)
	state, err := s.GetWorkflowExecutionInfo(ctx, domainID, execution)
	require.NoError(t, err)
	next := state.ExecutionInfo.NextEventID

	req := func(nextEventID int64) *persistence.DeleteWorkflowExecutionRequest {
		return &persistence.DeleteWorkflowExecutionRequest{DomainID: domainID, WorkflowID: execution.WorkflowID, RunID: execution.RunID, ShardID: common.IntPtr(s.ShardInfo.ShardID), NextEventID: &nextEventID}
	}
	getReq := &persistence.GetWorkflowExecutionRequest{DomainID: domainID, Execution: execution, ShardID: common.IntPtr(s.ShardInfo.ShardID), RangeID: s.ShardInfo.RangeID}

	assert.IsType(t, &persistence.ConditionFailedError{}, mgr.DeleteWorkflowExecution(ctx, req(next+1)), "the row moved on since it was read")
	_, err = mgr.GetWorkflowExecution(ctx, getReq)
	require.NoError(t, err, "a failed conditional delete leaves the row")

	require.NoError(t, mgr.DeleteWorkflowExecution(ctx, req(next)))
	_, err = mgr.GetWorkflowExecution(ctx, getReq)
	assert.IsType(t, &types.EntityNotExistsError{}, err)

	assert.IsType(t, &types.EntityNotExistsError{}, mgr.DeleteWorkflowExecution(ctx, req(next)), "the row is gone")
}
