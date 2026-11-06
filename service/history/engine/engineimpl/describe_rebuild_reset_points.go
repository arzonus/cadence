package engineimpl

import (
	"context"
	"time"

	"github.com/uber/cadence/common/definition"
	"github.com/uber/cadence/common/log/tag"
	"github.com/uber/cadence/common/types"
	"github.com/uber/cadence/service/history/execution"
	"github.com/uber/cadence/service/history/shard"
)

// rebuildAutoResetPoints replays history through the regular state builder, so the points come
// out as the live mutable state computed them (except CreatedTimeNano, which is replay time).
// Cost: reads the whole run's history, one read per history page.
func rebuildAutoResetPoints(
	ctx context.Context,
	shardCtx shard.Context,
	domainID string,
	ms execution.MutableState,
) (*types.ResetPoints, error) {
	vh, err := ms.GetVersionHistories().GetCurrentVersionHistory()
	if err != nil {
		return nil, err
	}
	lastItem, err := vh.GetLastItem()
	if err != nil {
		return nil, err
	}
	info := ms.GetExecutionInfo()
	id := definition.NewWorkflowIdentifier(domainID, info.WorkflowID, info.RunID)
	branchToken := vh.GetBranchToken()
	rebuilt, _, err := execution.NewStateRebuilder(shardCtx, shardCtx.GetLogger()).Rebuild(
		ctx, time.Now(), id, branchToken, lastItem.EventID, lastItem.Version, id,
		func() ([]byte, error) { return branchToken, nil }, "",
	)
	if err != nil {
		return nil, err
	}
	return rebuilt.GetExecutionInfo().AutoResetPoints, nil
}

// restoreDroppedResetPoints rebuilds the points of a run that was read from closed_executions, which drops them.
// The points are kept on the cached state and the marker is cleared, so history is replayed once per cache entry.
// It is best effort: Describe worked before warm storage, so an unreadable history mustn't fail it now.
func (e *historyEngineImpl) restoreDroppedResetPoints(
	ctx context.Context,
	domainID string,
	ms execution.MutableState,
	described *types.WorkflowExecutionInfo,
) {
	info := ms.GetExecutionInfo()
	if !info.AutoResetPointsDropped {
		return
	}
	points, err := rebuildAutoResetPoints(ctx, e.shard, domainID, ms)
	if err != nil {
		e.logger.Warn("could not rebuild auto-reset points for a closed run", tag.WorkflowDomainID(domainID), tag.Error(err))
		return
	}
	info.AutoResetPoints, info.AutoResetPointsDropped = points, false
	described.AutoResetPoints = points
}
