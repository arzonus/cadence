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

// With the flag off, a host that hasn't heard of the first move must find Warm rows on a missed read, and a
// delete must remove the closed row too: the sentinel is read on every miss until it is found. A failing read
// (no table) is not repeated within the retry interval.
func TestMissedReadFindsSentinel(t *testing.T) {
	s, e := newTestStore(t, false, nil)
	s.inUse.Store(false)
	s.sentinelWritten.Store(false)
	ctx := context.Background()
	gomock.InOrder(
		// not found: the miss is returned as is, and the next one reads the sentinel again
		e.closed.EXPECT().GetClosedExecution(gomock.Any(), sentinelKey).Return(nil, errNotExists),
		e.closed.EXPECT().GetClosedExecution(gomock.Any(), sentinelKey).Return(&persistence.DataBlob{}, nil),
		e.closed.EXPECT().GetClosedExecution(gomock.Any(), e.key).Return(e.slimBlob(t), nil),
		e.closed.EXPECT().DeleteClosedExecution(gomock.Any(), e.key).Return(nil),
	)
	e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), e.getRequest()).Return(nil, errNotExists).Times(2)
	e.inner.EXPECT().DeleteWorkflowExecution(gomock.Any(), e.deleteRequest()).Return(nil)

	_, err := s.GetWorkflowExecution(ctx, e.getRequest())
	assert.Equal(t, errNotExists, err)
	resp, err := s.GetWorkflowExecution(ctx, e.getRequest())
	require.NoError(t, err)
	assert.Equal(t, expectedSlimState(t, e.full), resp.State)
	require.NoError(t, s.DeleteWorkflowExecution(ctx, e.deleteRequest()))
}

func TestFailedSentinelReadIsNotRepeatedWithinInterval(t *testing.T) {
	s := newUnprobedStore(t, errDB, 2) // a third read fails the test
	ctx := context.Background()
	domainID := testKey().DomainID
	assert.False(t, s.fallbackOn(ctx, domainID))
	assert.False(t, s.fallbackOn(ctx, domainID), "within the interval")
	s.probeFailedAt.Store(time.Now().Add(-sentinelRetryInterval).UnixNano())
	assert.False(t, s.fallbackOn(ctx, domainID), "after the interval")
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
