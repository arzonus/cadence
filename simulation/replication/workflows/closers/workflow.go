// Package closers has workflows that end in a specific way, to cover every close status in the replication simulation.
package closers

import (
	"errors"

	"go.uber.org/cadence/workflow"

	"github.com/uber/cadence/simulation/replication/types"
)

// FailWorkflow fails right away.
func FailWorkflow(ctx workflow.Context, input types.WorkflowInput) (types.WorkflowOutput, error) {
	return types.WorkflowOutput{}, errors.New("failing on purpose")
}

// ContinueAsNewWorkflow continues as new input.ActivityCount more times, then completes. ActivityCount doubles as the remaining count.
func ContinueAsNewWorkflow(ctx workflow.Context, input types.WorkflowInput) (types.WorkflowOutput, error) {
	if input.ActivityCount <= 0 {
		return types.WorkflowOutput{}, nil
	}
	input.ActivityCount--
	return types.WorkflowOutput{}, workflow.NewContinueAsNewError(ctx, ContinueAsNewWorkflow, input)
}
