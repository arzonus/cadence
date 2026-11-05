package warmstorage

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/uber/cadence/common/constants"
	"github.com/uber/cadence/common/persistence"
	"github.com/uber/cadence/common/types"
)

// With the flag off, a host that missed the sentinel at startup (the probe failed, or no run had been moved
// yet) must still find Warm rows: a missed read re-probes the sentinel once the throttle interval has passed.
func TestMissedReadReprobesSentinel(t *testing.T) {
	for name, startupProbe := range map[string]error{
		"startup probe failed":               errDB,
		"host started before the first move": errNotExists,
	} {
		t.Run(name, func(t *testing.T) {
			s, e := newTestStore(t, false, startupProbe)
			ctx := context.Background()

			// within the interval: no extra query, the miss is returned as is
			e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), e.getRequest()).Return(nil, errNotExists)
			_, err := s.GetWorkflowExecution(ctx, e.getRequest())
			assert.Equal(t, errNotExists, err)

			s.probedAt.Store(time.Now().Add(-sentinelReprobeInterval).UnixNano())
			gomock.InOrder(
				e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), e.getRequest()).Return(nil, errNotExists),
				e.closed.EXPECT().GetClosedExecution(gomock.Any(), sentinelKey).Return(&persistence.DataBlob{}, nil),
				e.closed.EXPECT().GetClosedExecution(gomock.Any(), e.key).Return(e.slimBlob(t), nil),
			)
			resp, err := s.GetWorkflowExecution(ctx, e.getRequest())
			require.NoError(t, err)
			assert.Equal(t, expectedSlimState(t, e.full), resp.State)
			assert.True(t, s.inUse.Load())
		})
	}
}

// toBlob accepts version histories whose current branch has no items, but fromBlob (GetLastItem) rejects them:
// the move would delete the executions row and every later read of the run would fail.
func TestMoveWithUnreadableSlimRowKeepsRunHot(t *testing.T) {
	s, e := newTestStore(t, true, nil)
	blob, err := persistence.NewPayloadSerializer().SerializeVersionHistories(&types.VersionHistories{
		Histories: []*types.VersionHistory{{BranchToken: []byte("b")}},
	}, constants.EncodingTypeThriftRW)
	require.NoError(t, err)
	e.full.VersionHistories = blob
	e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), e.getRequest()).Return(&persistence.InternalGetWorkflowExecutionResponse{State: e.full}, nil)
	// strict mocks: any InsertClosedExecution / DeleteWorkflowExecution call fails the test
	assert.NoError(t, s.MoveToClosedExecutions(context.Background(), e.deleteRequest()))
	assert.EqualValues(t, 1, e.counter("warm_storage_move_failures"))
}

// NextEventID is re-derived from the last version-history item on read. If the hot row disagrees, the move
// would silently change it, so it refuses and keeps the run hot.
func TestMoveWithNextEventIDMismatchKeepsRunHot(t *testing.T) {
	s, e := newTestStore(t, true, nil)
	e.full.ExecutionInfo.NextEventID = lastEventID + 5
	e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), e.getRequest()).Return(&persistence.InternalGetWorkflowExecutionResponse{State: e.full}, nil)
	assert.NoError(t, s.MoveToClosedExecutions(context.Background(), e.deleteRequest()))
	assert.EqualValues(t, 1, e.counter("warm_storage_move_failures"))
}

func TestAdversarial_MoveSkipsZombie(t *testing.T) {
	s, e := newTestStore(t, true, nil)
	e.full.ExecutionInfo.State = persistence.WorkflowStateZombie
	e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), e.getRequest()).Return(&persistence.InternalGetWorkflowExecutionResponse{State: e.full}, nil)
	assert.NoError(t, s.MoveToClosedExecutions(context.Background(), e.deleteRequest()))
	assert.Zero(t, e.counter("warm_storage_move_failures"))
}

// Crash window: slim row written, delete fails; the retried close task must converge (upsert, delete).
func TestAdversarial_DeleteFailsThenRetrySucceeds(t *testing.T) {
	s, e := newTestStore(t, true, nil)
	ctx := context.Background()
	e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), e.getRequest()).Return(&persistence.InternalGetWorkflowExecutionResponse{State: e.full}, nil).Times(2)
	e.closed.EXPECT().InsertClosedExecution(gomock.Any(), e.key, gomock.Any()).Return(nil).Times(2)
	gomock.InOrder(
		e.inner.EXPECT().DeleteWorkflowExecution(gomock.Any(), e.deleteRequest()).Return(errDB),
		e.inner.EXPECT().DeleteWorkflowExecution(gomock.Any(), e.deleteRequest()).Return(nil),
	)
	require.NoError(t, s.MoveToClosedExecutions(ctx, e.deleteRequest()))
	require.NoError(t, s.MoveToClosedExecutions(ctx, e.deleteRequest()))
	assert.EqualValues(t, 1, e.counter("warm_storage_moves"))
}

// Delete when the closed delete fails: the error is returned (retention retries) and the hot delete is idempotent.
func TestAdversarial_DeleteClosedFailsIsRetryable(t *testing.T) {
	s, e := newTestStore(t, true, nil)
	ctx := context.Background()
	e.inner.EXPECT().DeleteWorkflowExecution(gomock.Any(), e.deleteRequest()).Return(nil).Times(2)
	gomock.InOrder(
		e.closed.EXPECT().DeleteClosedExecution(gomock.Any(), e.key).Return(errDB),
		e.closed.EXPECT().DeleteClosedExecution(gomock.Any(), e.key).Return(nil),
	)
	assert.Error(t, s.DeleteWorkflowExecution(ctx, e.deleteRequest()))
	assert.NoError(t, s.DeleteWorkflowExecution(ctx, e.deleteRequest()))
}

// fromBlob on a sentinel-shaped row must return an error, never panic or produce state.
func TestAdversarial_FromBlobOnSentinelRowErrors(t *testing.T) {
	_, err := fromBlob(testParser(t), persistence.NewPayloadSerializer(), sentinelKey,
		persistence.NewDataBlob([]byte{0}, constants.EncodingTypeEmpty))
	assert.Error(t, err)
}
