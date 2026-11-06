package warmstorage

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/uber/cadence/common/persistence"
)

// Crash window: slim row written, delete fails; the retried close task must converge (upsert, delete).
func TestAdversarial_DeleteFailsThenRetrySucceeds(t *testing.T) {
	s, e := newTestStore(t, "on")
	ctx := context.Background()
	e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), e.getRequest()).Return(&persistence.InternalGetWorkflowExecutionResponse{State: e.full}, nil).Times(2)
	e.closed.EXPECT().InsertClosedExecution(gomock.Any(), e.key, gomock.Any()).Return(nil).Times(2)
	gomock.InOrder(
		e.inner.EXPECT().DeleteWorkflowExecution(gomock.Any(), e.moveDeleteRequest()).Return(errDB),
		e.inner.EXPECT().DeleteWorkflowExecution(gomock.Any(), e.moveDeleteRequest()).Return(nil),
	)
	require.NoError(t, s.MoveToClosedExecutions(ctx, e.deleteRequest()))
	require.NoError(t, s.MoveToClosedExecutions(ctx, e.deleteRequest()))
	assert.EqualValues(t, 1, e.counter("warm_storage_moves"))
}
