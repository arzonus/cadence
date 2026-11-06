package warmstorage

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/uber/cadence/common"
	"github.com/uber/cadence/common/persistence"
	"github.com/uber/cadence/common/types"
)

func TestUpdateWorkflowExecutionRestoresMovedRun(t *testing.T) {
	notThere := &types.EntityNotExistsError{Message: "update on a missing row"}
	tests := map[string]struct {
		firstErr error
		setup    func(e *testEnv, req *persistence.InternalUpdateWorkflowExecutionRequest)
		wantErr  error
		wantRows int64
	}{
		"update lands: nothing is restored": {
			setup: func(e *testEnv, req *persistence.InternalUpdateWorkflowExecutionRequest) {},
		},
		"other error is returned as is": {
			firstErr: errDB, wantErr: errDB,
			setup: func(e *testEnv, req *persistence.InternalUpdateWorkflowExecutionRequest) {},
		},
		"moved run: row is put back, update retried": {
			firstErr: notThere, wantRows: 1,
			setup: func(e *testEnv, req *persistence.InternalUpdateWorkflowExecutionRequest) {
				e.closed.EXPECT().GetClosedExecution(gomock.Any(), e.key).Return(e.slimBlob(t), nil)
				e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), gomock.Any()).Return(nil, errNotExists)
				e.inner.EXPECT().CreateWorkflowExecution(gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, r *persistence.InternalCreateWorkflowExecutionRequest) (*persistence.CreateWorkflowExecutionResponse, error) {
						assert.Equal(t, persistence.CreateWorkflowModeZombie, r.Mode)
						assert.Equal(t, int64(9), r.RangeID)
						assert.Equal(t, persistence.WorkflowStateCompleted, r.NewWorkflowSnapshot.ExecutionInfo.State)
						assert.Equal(t, int64(lastEventID+1), r.NewWorkflowSnapshot.ExecutionInfo.NextEventID)
						return &persistence.CreateWorkflowExecutionResponse{}, nil
					})
				e.inner.EXPECT().UpdateWorkflowExecution(gomock.Any(), req).Return(nil)
			},
		},
		"Cassandra reports a missing row as a failed condition": {
			firstErr: &persistence.ConditionFailedError{}, wantRows: 1,
			setup: func(e *testEnv, req *persistence.InternalUpdateWorkflowExecutionRequest) {
				e.closed.EXPECT().GetClosedExecution(gomock.Any(), e.key).Return(e.slimBlob(t), nil)
				e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), gomock.Any()).Return(nil, errNotExists)
				e.inner.EXPECT().CreateWorkflowExecution(gomock.Any(), gomock.Any()).Return(&persistence.CreateWorkflowExecutionResponse{}, nil)
				e.inner.EXPECT().UpdateWorkflowExecution(gomock.Any(), req).Return(nil)
			},
		},
		"another writer restored it first: update retried": {
			firstErr: notThere, wantRows: 1,
			setup: func(e *testEnv, req *persistence.InternalUpdateWorkflowExecutionRequest) {
				e.closed.EXPECT().GetClosedExecution(gomock.Any(), e.key).Return(e.slimBlob(t), nil)
				e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), gomock.Any()).Return(nil, errNotExists)
				e.inner.EXPECT().CreateWorkflowExecution(gomock.Any(), gomock.Any()).Return(nil, &persistence.WorkflowExecutionAlreadyStartedError{})
				e.inner.EXPECT().UpdateWorkflowExecution(gomock.Any(), req).Return(nil)
			},
		},
		"no slim row: the original error": {
			firstErr: notThere, wantErr: notThere,
			setup: func(e *testEnv, req *persistence.InternalUpdateWorkflowExecutionRequest) {
				e.closed.EXPECT().GetClosedExecution(gomock.Any(), e.key).Return(nil, errNotExists)
			},
		},
		"executions row exists (a real condition failure): the original error": {
			firstErr: &persistence.ConditionFailedError{}, wantErr: &persistence.ConditionFailedError{},
			setup: func(e *testEnv, req *persistence.InternalUpdateWorkflowExecutionRequest) {
				e.closed.EXPECT().GetClosedExecution(gomock.Any(), e.key).Return(e.slimBlob(t), nil)
				e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), gomock.Any()).Return(&persistence.InternalGetWorkflowExecutionResponse{State: e.full}, nil)
			},
		},
		"restoring fails: that error, no retry": {
			firstErr: notThere, wantErr: errDB,
			setup: func(e *testEnv, req *persistence.InternalUpdateWorkflowExecutionRequest) {
				e.closed.EXPECT().GetClosedExecution(gomock.Any(), e.key).Return(e.slimBlob(t), nil)
				e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), gomock.Any()).Return(nil, errNotExists)
				e.inner.EXPECT().CreateWorkflowExecution(gomock.Any(), gomock.Any()).Return(nil, errDB)
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			s, e := newTestStore(t, true, nil)
			req := &persistence.InternalUpdateWorkflowExecutionRequest{
				ShardID: common.IntPtr(7),
				RangeID: 9,
				UpdateWorkflowMutation: persistence.InternalWorkflowMutation{ExecutionInfo: &persistence.InternalWorkflowExecutionInfo{
					DomainID: e.key.DomainID, WorkflowID: e.key.WorkflowID, RunID: e.key.RunID,
				}},
			}
			e.inner.EXPECT().UpdateWorkflowExecution(gomock.Any(), req).Return(tc.firstErr)
			tc.setup(e, req)
			assert.Equal(t, tc.wantErr, s.UpdateWorkflowExecution(context.Background(), req))
			assert.Equal(t, tc.wantRows, e.counter("warm_storage_rehydrates"))
		})
	}
}

func TestLazyClosedStore(t *testing.T) {
	ctx := context.Background()
	key := testKey()
	inner := persistence.NewMockClosedExecutionStore(gomock.NewController(t))
	opens := 0
	lazy := NewLazyClosedStore(func() (persistence.ClosedExecutionStore, error) {
		opens++
		if opens == 1 {
			return nil, errDB
		}
		return inner, nil
	})

	assert.Zero(t, opens, "nothing is opened at construction")
	lazy.Close() // closing an unopened store is a no-op

	_, err := lazy.GetClosedExecution(ctx, key)
	assert.Equal(t, errDB, err, "a failed open is reported and retried")

	inner.EXPECT().GetClosedExecution(ctx, key).Return(&persistence.DataBlob{}, nil)
	inner.EXPECT().InsertClosedExecution(ctx, key, gomock.Any()).Return(nil)
	inner.EXPECT().DeleteClosedExecution(ctx, key).Return(nil)
	inner.EXPECT().Close()
	_, err = lazy.GetClosedExecution(ctx, key)
	require.NoError(t, err)
	require.NoError(t, lazy.InsertClosedExecution(ctx, key, &persistence.DataBlob{}))
	require.NoError(t, lazy.DeleteClosedExecution(ctx, key))
	assert.Equal(t, 2, opens, "opened once it works")
	lazy.Close()
}
