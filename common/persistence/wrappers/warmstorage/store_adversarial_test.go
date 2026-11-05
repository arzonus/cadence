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

// fromBlob on a sentinel-shaped row must return an error, never panic or produce state.
func TestAdversarial_FromBlobOnSentinelRowErrors(t *testing.T) {
	_, err := fromBlob(testParser(t), persistence.NewPayloadSerializer(), sentinelKey,
		persistence.NewDataBlob([]byte{0}, constants.EncodingTypeEmpty))
	assert.Error(t, err)
}
