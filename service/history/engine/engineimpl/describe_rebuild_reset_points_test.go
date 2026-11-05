package engineimpl

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/uber/cadence/common/constants"
	"github.com/uber/cadence/common/persistence"
	"github.com/uber/cadence/common/types"
	"github.com/uber/cadence/service/history/config"
	historyConstants "github.com/uber/cadence/service/history/constants"
	"github.com/uber/cadence/service/history/events"
	"github.com/uber/cadence/service/history/execution"
	"github.com/uber/cadence/service/history/shard"
	test "github.com/uber/cadence/service/history/testing"
)

func TestRebuildAutoResetPoints(t *testing.T) {
	ctrl := gomock.NewController(t)
	shardCtx := shard.NewTestContext(t, ctrl, &persistence.ShardInfo{ShardID: 0, RangeID: 1}, config.NewForTest())
	defer shardCtx.Finish(t)
	shardCtx.SetEventsCache(events.NewCache(shardCtx.GetShardID(), shardCtx.GetHistoryManager(), shardCtx.GetConfig(),
		shardCtx.GetLogger(), shardCtx.GetMetricsClient(), shardCtx.GetDomainCache()))
	domainID := historyConstants.TestDomainID
	shardCtx.Resource.DomainCache.EXPECT().GetDomainByID(domainID).Return(historyConstants.TestGlobalDomainEntry, nil).AnyTimes()
	shardCtx.Resource.DomainCache.EXPECT().GetDomainName(domainID).Return(historyConstants.TestDomainName, nil).AnyTimes()

	// live mutable state: three decisions, the third repeats the first checksum
	_, ms, err := test.StartWorkflow(t, shardCtx, domainID)
	require.NoError(t, err)
	for _, checksum := range []string{"bin-a", "bin-b", "bin-a"} {
		di := test.AddDecisionTaskScheduledEvent(ms)
		started := test.AddDecisionTaskStartedEvent(ms, di.ScheduleID, "some random task list", "req")
		_, err := ms.AddDecisionTaskCompletedEvent(di.ScheduleID, started.ID, &types.RespondDecisionTaskCompletedRequest{
			BinaryChecksum: checksum,
		}, constants.DefaultHistoryMaxAutoResetPoints)
		require.NoError(t, err)
		require.NoError(t, ms.FlushBufferedEvents())
	}
	test.AddCompleteWorkflowEvent(ms, ms.GetNextEventID()-1, nil)
	want := ms.GetExecutionInfo().AutoResetPoints
	require.Len(t, want.Points, 2)

	_, eventBatches, err := ms.CloseTransactionAsMutation(time.Now(), execution.TransactionPolicyActive)
	require.NoError(t, err)
	var history []*types.History
	for _, b := range eventBatches {
		history = append(history, &types.History{Events: b.Events})
	}
	shardCtx.Resource.HistoryMgr.On("ReadHistoryBranchByBatch", mock.Anything, mock.Anything).
		Return(&persistence.ReadHistoryBranchByBatchResponse{History: history}, nil).Once()

	got, err := rebuildAutoResetPoints(context.Background(), shardCtx, domainID, ms)
	require.NoError(t, err)
	require.Len(t, got.Points, len(want.Points))
	for i, p := range got.Points {
		w := want.Points[i]
		// created time is replay time, so it is not comparable
		require.Equal(t, w.BinaryChecksum, p.BinaryChecksum)
		require.Equal(t, w.RunID, p.RunID)
		require.Equal(t, w.FirstDecisionCompletedID, p.FirstDecisionCompletedID)
		require.Equal(t, w.Resettable, p.Resettable)
	}
}
