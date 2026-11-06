package warmstorage

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/uber/cadence/common/constants"
	"github.com/uber/cadence/common/persistence"
	"github.com/uber/cadence/common/types"
)

func TestHotReadOfRestoredRunIsMarked(t *testing.T) {
	tests := map[string]struct {
		state    int
		points   *persistence.DataBlob
		mode     string
		slimRow  bool // whether the closed store has the row
		readSlim bool // whether the decorator is expected to look
		wantMark bool
	}{
		"closed, no points, slim row: marked":            {persistence.WorkflowStateCompleted, nil, "on", true, true, true},
		"closed, no points, no slim row: not marked":     {persistence.WorkflowStateCompleted, nil, "on", false, true, false},
		"closed, empty points stored by a write: marked": {persistence.WorkflowStateCompleted, emptyPoints(t), "on", true, true, true},
		"closed with points: slim row not read":          {persistence.WorkflowStateCompleted, withPoints(t), "on", true, false, false},
		"open run: slim row not read":                    {persistence.WorkflowStateRunning, nil, "on", true, false, false},
		"read_only: closed, no points, slim row: marked": {persistence.WorkflowStateCompleted, nil, "read_only", true, true, true},
		"off: slim row not read":                         {persistence.WorkflowStateCompleted, nil, "off", true, false, false},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			s, e := newTestStore(t, tc.mode)
			state := *e.full
			info := *state.ExecutionInfo
			info.State, info.AutoResetPoints = tc.state, tc.points
			state.ExecutionInfo = &info
			e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), e.getRequest()).Return(&persistence.InternalGetWorkflowExecutionResponse{State: &state}, nil)
			if tc.readSlim {
				if tc.slimRow {
					e.closed.EXPECT().GetClosedExecution(gomock.Any(), e.key).Return(e.slimBlob(t), nil)
				} else {
					e.closed.EXPECT().GetClosedExecution(gomock.Any(), e.key).Return(nil, errNotExists)
				}
			}
			resp, err := s.GetWorkflowExecution(context.Background(), e.getRequest())
			require.NoError(t, err)
			assert.Equal(t, tc.wantMark, resp.State.ExecutionInfo.AutoResetPointsDropped)
		})
	}
}

func emptyPoints(t *testing.T) *persistence.DataBlob {
	blob, err := persistence.NewPayloadSerializer().SerializeResetPoints(nil, constants.EncodingTypeThriftRW)
	require.NoError(t, err)
	return blob
}

func withPoints(t *testing.T) *persistence.DataBlob {
	blob, err := persistence.NewPayloadSerializer().SerializeResetPoints(&types.ResetPoints{Points: []*types.ResetPointInfo{{BinaryChecksum: "bc"}}}, constants.EncodingTypeThriftRW)
	require.NoError(t, err)
	return blob
}
