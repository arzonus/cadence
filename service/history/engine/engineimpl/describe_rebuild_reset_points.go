package engineimpl

import (
	"context"
	"time"

	"github.com/uber/cadence/common/definition"
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
