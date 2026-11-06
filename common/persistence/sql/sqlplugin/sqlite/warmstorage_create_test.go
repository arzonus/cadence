//go:build !race

package sqlite

import (
	"context"
	"testing"

	"github.com/pborman/uuid"
	"github.com/stretchr/testify/require"

	"github.com/uber/cadence/common/persistence"
	"github.com/uber/cadence/common/types"
)

// TestCreateWithMovedCurrentRun: the current_executions row of a moved run has no executions row, so creating a
// new run for the workflow ID must still see it and report it as already started instead of a unique violation.
func TestCreateWithMovedCurrentRun(t *testing.T) {
	e := newWarmEnv(t)
	ctx := context.Background()
	e.create(t)
	e.closeRun(t)
	require.NoError(t, e.base.ExecutionManager.MoveToClosedExecutions(ctx, e.req()))

	we2 := types.WorkflowExecution{WorkflowID: e.we.WorkflowID, RunID: uuid.New()}
	_, err := e.base.CreateWorkflowExecutionWithBranchToken(ctx, e.domainID, we2, "tl", "wt", 100, 10, nil, 3, 0, 2, []byte("branch"), nil, nil)
	var started *persistence.WorkflowExecutionAlreadyStartedError
	require.ErrorAs(t, err, &started)
	require.Equal(t, e.we.RunID, started.RunID)
	require.Equal(t, persistence.WorkflowStateCompleted, started.State)
}
