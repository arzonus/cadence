package persistence

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRestoreClosedRunValidators(t *testing.T) {
	tests := []struct {
		name         string
		state        int
		closeStatus  int
		restore      bool
		wantModeErr  bool
		wantStateErr bool
	}{
		{"completed run without the flag: rejected by both, as on master", WorkflowStateCompleted, WorkflowCloseStatusCompleted, false, true, true},
		{"completed run with the flag: accepted by both", WorkflowStateCompleted, WorkflowCloseStatusCompleted, true, false, false},
		{"completed without a close status, with the flag: rejected", WorkflowStateCompleted, WorkflowCloseStatusNone, true, false, true},
		{"running with the flag: rejected", WorkflowStateRunning, WorkflowCloseStatusNone, true, true, false},
		{"zombie with the flag: accepted", WorkflowStateZombie, WorkflowCloseStatusNone, true, false, false},
		{"zombie without the flag: accepted", WorkflowStateZombie, WorkflowCloseStatusNone, false, false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := InternalWorkflowSnapshot{ExecutionInfo: &InternalWorkflowExecutionInfo{State: tc.state}, RestoreClosedRun: tc.restore}
			assert.Equal(t, tc.wantModeErr, ValidateCreateWorkflowModeState(CreateWorkflowModeZombie, snapshot) != nil, "mode validator")
			assert.Equal(t, tc.wantStateErr, ValidateCreateWorkflowStateCloseStatus(tc.state, tc.closeStatus, tc.restore) != nil, "state validator")
		})
	}
}
