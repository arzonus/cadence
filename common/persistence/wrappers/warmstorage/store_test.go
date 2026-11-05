package warmstorage

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/tally"
	"go.uber.org/mock/gomock"

	"github.com/uber/cadence/common"
	"github.com/uber/cadence/common/dynamicconfig/dynamicproperties"
	"github.com/uber/cadence/common/log/testlogger"
	"github.com/uber/cadence/common/metrics"
	"github.com/uber/cadence/common/persistence"
	"github.com/uber/cadence/common/types"
)

var (
	errNotExists = &types.EntityNotExistsError{Message: "missing"}
	errDB        = errors.New("db down")
)

type testEnv struct {
	inner  *persistence.MockExecutionStore
	closed *persistence.MockClosedExecutionStore
	scope  tally.TestScope
	key    persistence.ClosedExecutionKey
	full   *persistence.InternalWorkflowMutableState
}

// newTestStore builds the decorator. sentinel is what the startup probe returns.
func newTestStore(t *testing.T, flagOn bool, sentinel error) (*store, *testEnv) {
	ctrl := gomock.NewController(t)
	env := &testEnv{
		inner:  persistence.NewMockExecutionStore(ctrl),
		closed: persistence.NewMockClosedExecutionStore(ctrl),
		scope:  tally.NewTestScope("", nil),
		key:    testKey(),
	}
	env.full = closedState(t, env.key)
	env.closed.EXPECT().GetClosedExecution(gomock.Any(), sentinelKey).Return(nil, sentinel)
	dc := persistence.NewDefaultDynamicConfiguration()
	dc.EnableWarmStorage = dynamicproperties.GetBoolPropertyFnFilteredByDomainID(flagOn)
	s := NewExecutionStore(env.inner, env.closed, testParser(t), dc, testlogger.New(t),
		metrics.NewClient(env.scope, metrics.History, metrics.MigrationConfig{})).(*store)
	return s, env
}

func (e *testEnv) getRequest() *persistence.InternalGetWorkflowExecutionRequest {
	return &persistence.InternalGetWorkflowExecutionRequest{
		ShardID:   common.IntPtr(7),
		DomainID:  e.key.DomainID,
		Execution: types.WorkflowExecution{WorkflowID: e.key.WorkflowID, RunID: e.key.RunID},
	}
}

func (e *testEnv) deleteRequest() *persistence.DeleteWorkflowExecutionRequest {
	return &persistence.DeleteWorkflowExecutionRequest{ShardID: common.IntPtr(7), DomainID: e.key.DomainID, WorkflowID: e.key.WorkflowID, RunID: e.key.RunID}
}

func (e *testEnv) slimBlob(t *testing.T) *persistence.DataBlob {
	blob, err := toBlob(testParser(t), e.full)
	require.NoError(t, err)
	return blob
}

func (e *testEnv) counter(name string) int64 {
	var total int64
	for _, c := range e.scope.Snapshot().Counters() {
		if c.Name() == name {
			total += c.Value()
		}
	}
	return total
}

func TestNewExecutionStoreProbesSentinel(t *testing.T) {
	tests := map[string]struct {
		sentinel  error
		wantInUse bool
	}{
		"sentinel found: cluster has Warm rows": {sentinel: nil, wantInUse: true},
		"sentinel missing: never enabled":       {sentinel: errNotExists, wantInUse: false},
		"table missing: never enabled":          {sentinel: errDB, wantInUse: false},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			s, _ := newTestStore(t, false, tc.sentinel)
			assert.Equal(t, tc.wantInUse, s.inUse.Load())
		})
	}
}

func TestGetWorkflowExecution(t *testing.T) {
	tests := map[string]struct {
		flagOn    bool
		sentinel  error
		setup     func(t *testing.T, e *testEnv)
		wantSlim  bool
		wantErr   error
		wantFound bool
		wantHits  int64
	}{
		"open or not yet moved run comes from executions, closed table untouched": {
			flagOn: true, sentinel: errNotExists,
			setup: func(t *testing.T, e *testEnv) {
				e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), e.getRequest()).Return(&persistence.InternalGetWorkflowExecutionResponse{State: e.full}, nil)
			},
			wantFound: true,
		},
		"flag off and never enabled: miss is returned as is": {
			flagOn: false, sentinel: errNotExists,
			setup: func(t *testing.T, e *testEnv) {
				e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), e.getRequest()).Return(nil, errNotExists)
			},
			wantErr: errNotExists,
		},
		"flag on: Warm run is read from closed_executions": {
			flagOn: true, sentinel: errNotExists,
			setup: func(t *testing.T, e *testEnv) {
				e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), e.getRequest()).Return(nil, errNotExists)
				e.closed.EXPECT().GetClosedExecution(gomock.Any(), e.key).Return(e.slimBlob(t), nil)
			},
			wantSlim: true, wantHits: 1,
		},
		"flag turned off after Warm rows were written: still readable": {
			flagOn: false, sentinel: nil,
			setup: func(t *testing.T, e *testEnv) {
				e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), e.getRequest()).Return(nil, errNotExists)
				e.closed.EXPECT().GetClosedExecution(gomock.Any(), e.key).Return(e.slimBlob(t), nil)
			},
			wantSlim: true, wantHits: 1,
		},
		"missing run: original not-exists error": {
			flagOn: true, sentinel: errNotExists,
			setup: func(t *testing.T, e *testEnv) {
				e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), e.getRequest()).Return(nil, errNotExists)
				e.closed.EXPECT().GetClosedExecution(gomock.Any(), e.key).Return(nil, &types.EntityNotExistsError{Message: "closed"})
			},
			wantErr: errNotExists,
		},
		"executions error other than not-exists is not masked": {
			flagOn: true, sentinel: errNotExists,
			setup: func(t *testing.T, e *testEnv) {
				e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), e.getRequest()).Return(nil, errDB)
			},
			wantErr: errDB,
		},
		"closed_executions error is returned": {
			flagOn: true, sentinel: errNotExists,
			setup: func(t *testing.T, e *testEnv) {
				e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), e.getRequest()).Return(nil, errNotExists)
				e.closed.EXPECT().GetClosedExecution(gomock.Any(), e.key).Return(nil, errDB)
			},
			wantErr: errDB,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			s, e := newTestStore(t, tc.flagOn, tc.sentinel)
			tc.setup(t, e)
			resp, err := s.GetWorkflowExecution(context.Background(), e.getRequest())
			assert.Equal(t, tc.wantErr, err)
			switch {
			case tc.wantSlim:
				require.NotNil(t, resp)
				assert.Equal(t, expectedSlimState(t, e.full), resp.State)
			case tc.wantFound:
				assert.Equal(t, e.full, resp.State)
			default:
				assert.Nil(t, resp)
			}
			assert.Equal(t, tc.wantHits, e.counter("warm_storage_fallback_hits"))
		})
	}
}

func TestIsWorkflowExecutionExists(t *testing.T) {
	tests := map[string]struct {
		flagOn  bool
		setup   func(e *testEnv)
		want    bool
		wantErr error
	}{
		"in executions": {
			flagOn: true,
			setup: func(e *testEnv) {
				e.inner.EXPECT().IsWorkflowExecutionExists(gomock.Any(), gomock.Any()).Return(&persistence.IsWorkflowExecutionExistsResponse{Exists: true}, nil)
			},
			want: true,
		},
		"Warm run exists, so the fixer won't treat its current row as orphaned": {
			flagOn: true,
			setup: func(e *testEnv) {
				e.inner.EXPECT().IsWorkflowExecutionExists(gomock.Any(), gomock.Any()).Return(&persistence.IsWorkflowExecutionExistsResponse{}, nil)
				e.closed.EXPECT().GetClosedExecution(gomock.Any(), e.key).Return(&persistence.DataBlob{}, nil)
			},
			want: true,
		},
		"missing everywhere": {
			flagOn: true,
			setup: func(e *testEnv) {
				e.inner.EXPECT().IsWorkflowExecutionExists(gomock.Any(), gomock.Any()).Return(&persistence.IsWorkflowExecutionExistsResponse{}, nil)
				e.closed.EXPECT().GetClosedExecution(gomock.Any(), e.key).Return(nil, errNotExists)
			},
		},
		"flag off: no closed lookup": {
			setup: func(e *testEnv) {
				e.inner.EXPECT().IsWorkflowExecutionExists(gomock.Any(), gomock.Any()).Return(&persistence.IsWorkflowExecutionExistsResponse{}, nil)
			},
		},
		"store error (SQL doesn't implement it) passes through": {
			flagOn: true,
			setup: func(e *testEnv) {
				e.inner.EXPECT().IsWorkflowExecutionExists(gomock.Any(), gomock.Any()).Return(nil, errDB)
			},
			wantErr: errDB,
		},
		"closed lookup error": {
			flagOn: true,
			setup: func(e *testEnv) {
				e.inner.EXPECT().IsWorkflowExecutionExists(gomock.Any(), gomock.Any()).Return(&persistence.IsWorkflowExecutionExistsResponse{}, nil)
				e.closed.EXPECT().GetClosedExecution(gomock.Any(), e.key).Return(nil, errDB)
			},
			wantErr: errDB,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			s, e := newTestStore(t, tc.flagOn, errNotExists)
			tc.setup(e)
			resp, err := s.IsWorkflowExecutionExists(context.Background(), &persistence.IsWorkflowExecutionExistsRequest{
				DomainID: e.key.DomainID, WorkflowID: e.key.WorkflowID, RunID: e.key.RunID,
			})
			assert.Equal(t, tc.wantErr, err)
			if tc.wantErr == nil {
				assert.Equal(t, tc.want, resp.Exists)
			}
		})
	}
}

func TestDeleteWorkflowExecution(t *testing.T) {
	tests := map[string]struct {
		flagOn   bool
		sentinel error
		setup    func(e *testEnv)
		wantErr  error
	}{
		"flag off, never enabled: only executions": {
			sentinel: errNotExists,
			setup: func(e *testEnv) {
				e.inner.EXPECT().DeleteWorkflowExecution(gomock.Any(), e.deleteRequest()).Return(nil)
			},
		},
		"retention deletes both rows": {
			flagOn: true, sentinel: errNotExists,
			setup: func(e *testEnv) {
				gomock.InOrder(
					e.inner.EXPECT().DeleteWorkflowExecution(gomock.Any(), e.deleteRequest()).Return(nil),
					e.closed.EXPECT().DeleteClosedExecution(gomock.Any(), e.key).Return(nil),
				)
			},
		},
		"flag turned off after Warm rows were written: still deletes both": {
			sentinel: nil,
			setup: func(e *testEnv) {
				e.inner.EXPECT().DeleteWorkflowExecution(gomock.Any(), e.deleteRequest()).Return(nil)
				e.closed.EXPECT().DeleteClosedExecution(gomock.Any(), e.key).Return(nil)
			},
		},
		"executions delete fails: closed row kept for the retry": {
			flagOn: true, sentinel: errNotExists,
			setup: func(e *testEnv) {
				e.inner.EXPECT().DeleteWorkflowExecution(gomock.Any(), e.deleteRequest()).Return(errDB)
			},
			wantErr: errDB,
		},
		"closed delete fails: error so retention retries": {
			flagOn: true, sentinel: errNotExists,
			setup: func(e *testEnv) {
				e.inner.EXPECT().DeleteWorkflowExecution(gomock.Any(), e.deleteRequest()).Return(nil)
				e.closed.EXPECT().DeleteClosedExecution(gomock.Any(), e.key).Return(errDB)
			},
			wantErr: errDB,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			s, e := newTestStore(t, tc.flagOn, tc.sentinel)
			tc.setup(e)
			assert.Equal(t, tc.wantErr, s.DeleteWorkflowExecution(context.Background(), e.deleteRequest()))
		})
	}
}

func TestMoveToClosedExecutions(t *testing.T) {
	tests := map[string]struct {
		sentinel     error
		setup        func(t *testing.T, e *testEnv)
		wantMoves    int64
		wantFailures int64
		wantInUse    bool
	}{
		"moves: sentinel, then slim row, then delete": {
			sentinel: errNotExists,
			setup: func(t *testing.T, e *testEnv) {
				gomock.InOrder(
					e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), e.getRequest()).Return(&persistence.InternalGetWorkflowExecutionResponse{State: e.full}, nil),
					e.closed.EXPECT().InsertClosedExecution(gomock.Any(), sentinelKey, gomock.Any()).Return(nil),
					e.closed.EXPECT().InsertClosedExecution(gomock.Any(), e.key, e.slimBlob(t)).Return(nil),
					e.inner.EXPECT().DeleteWorkflowExecution(gomock.Any(), e.deleteRequest()).Return(nil),
				)
			},
			wantMoves: 1, wantInUse: true,
		},
		"sentinel already there: not rewritten": {
			sentinel: nil,
			setup: func(t *testing.T, e *testEnv) {
				gomock.InOrder(
					e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), e.getRequest()).Return(&persistence.InternalGetWorkflowExecutionResponse{State: e.full}, nil),
					e.closed.EXPECT().InsertClosedExecution(gomock.Any(), e.key, e.slimBlob(t)).Return(nil),
					e.inner.EXPECT().DeleteWorkflowExecution(gomock.Any(), e.deleteRequest()).Return(nil),
				)
			},
			wantMoves: 1, wantInUse: true,
		},
		"close task delivered again after the move: no-op": {
			sentinel: nil,
			setup: func(t *testing.T, e *testEnv) {
				e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), e.getRequest()).Return(nil, errNotExists)
			},
			wantInUse: true,
		},
		"open run (standby hasn't closed it yet) is skipped quietly": {
			sentinel: errNotExists,
			setup: func(t *testing.T, e *testEnv) {
				e.full.ExecutionInfo.State = persistence.WorkflowStateRunning
				e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), e.getRequest()).Return(&persistence.InternalGetWorkflowExecutionResponse{State: e.full}, nil)
			},
		},
		"legacy run without version histories stays hot": {
			sentinel: errNotExists,
			setup: func(t *testing.T, e *testEnv) {
				e.full.VersionHistories = nil
				e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), e.getRequest()).Return(&persistence.InternalGetWorkflowExecutionResponse{State: e.full}, nil)
			},
			wantFailures: 1,
		},
		"read fails: run stays hot, close task not failed": {
			sentinel: errNotExists,
			setup: func(t *testing.T, e *testEnv) {
				e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), e.getRequest()).Return(nil, errDB)
			},
			wantFailures: 1,
		},
		"table missing (sentinel write fails): run stays hot": {
			sentinel: errDB,
			setup: func(t *testing.T, e *testEnv) {
				e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), e.getRequest()).Return(&persistence.InternalGetWorkflowExecutionResponse{State: e.full}, nil)
				e.closed.EXPECT().InsertClosedExecution(gomock.Any(), sentinelKey, gomock.Any()).Return(errDB)
			},
			wantFailures: 1,
		},
		"slim write fails: executions row is NOT deleted": {
			sentinel: nil,
			setup: func(t *testing.T, e *testEnv) {
				e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), e.getRequest()).Return(&persistence.InternalGetWorkflowExecutionResponse{State: e.full}, nil)
				e.closed.EXPECT().InsertClosedExecution(gomock.Any(), e.key, gomock.Any()).Return(errDB)
			},
			wantFailures: 1, wantInUse: true,
		},
		"delete fails after slim write: both rows exist, reads prefer executions": {
			sentinel: nil,
			setup: func(t *testing.T, e *testEnv) {
				e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), e.getRequest()).Return(&persistence.InternalGetWorkflowExecutionResponse{State: e.full}, nil)
				e.closed.EXPECT().InsertClosedExecution(gomock.Any(), e.key, gomock.Any()).Return(nil)
				e.inner.EXPECT().DeleteWorkflowExecution(gomock.Any(), e.deleteRequest()).Return(errDB)
			},
			wantFailures: 1, wantInUse: true,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// flag off on purpose: the caller gates the move, and the decorator must not re-check it
			s, e := newTestStore(t, false, tc.sentinel)
			tc.setup(t, e)
			assert.NoError(t, s.MoveToClosedExecutions(context.Background(), e.deleteRequest()))
			assert.Equal(t, tc.wantMoves, e.counter("warm_storage_moves"))
			assert.Equal(t, tc.wantFailures, e.counter("warm_storage_move_failures"))
			assert.Equal(t, tc.wantInUse, s.inUse.Load())
		})
	}
}

func TestSentinelWrittenOnce(t *testing.T) {
	s, e := newTestStore(t, true, errNotExists)
	e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), gomock.Any()).Return(&persistence.InternalGetWorkflowExecutionResponse{State: e.full}, nil).Times(2)
	e.closed.EXPECT().InsertClosedExecution(gomock.Any(), sentinelKey, gomock.Any()).Return(nil).Times(1)
	e.closed.EXPECT().InsertClosedExecution(gomock.Any(), e.key, gomock.Any()).Return(nil).Times(2)
	e.inner.EXPECT().DeleteWorkflowExecution(gomock.Any(), gomock.Any()).Return(nil).Times(2)
	require.NoError(t, s.MoveToClosedExecutions(context.Background(), e.deleteRequest()))
	require.NoError(t, s.MoveToClosedExecutions(context.Background(), e.deleteRequest()))
}

// TestFlagOffIssuesSameCallsAsWithoutDecorator is the opt-in proof at the store boundary: in a cluster that
// never enabled warm storage, every overridden call reaches the wrapped store exactly once with the same
// request and the closed store sees nothing but the startup probe (strict mocks fail on any other call).
func TestFlagOffIssuesSameCallsAsWithoutDecorator(t *testing.T) {
	s, e := newTestStore(t, false, errNotExists)
	ctx := context.Background()
	getReq := e.getRequest()
	existsReq := &persistence.IsWorkflowExecutionExistsRequest{DomainID: e.key.DomainID, WorkflowID: e.key.WorkflowID, RunID: e.key.RunID}
	deleteReq := e.deleteRequest()
	updateReq := &persistence.InternalUpdateWorkflowExecutionRequest{RangeID: 1}

	gomock.InOrder(
		e.inner.EXPECT().GetWorkflowExecution(ctx, getReq).Return(&persistence.InternalGetWorkflowExecutionResponse{State: e.full}, nil),
		e.inner.EXPECT().GetWorkflowExecution(ctx, getReq).Return(nil, errNotExists),
		e.inner.EXPECT().IsWorkflowExecutionExists(ctx, existsReq).Return(&persistence.IsWorkflowExecutionExistsResponse{}, nil),
		e.inner.EXPECT().UpdateWorkflowExecution(ctx, updateReq).Return(nil),
		e.inner.EXPECT().DeleteWorkflowExecution(ctx, deleteReq).Return(nil),
	)

	resp, err := s.GetWorkflowExecution(ctx, getReq)
	require.NoError(t, err)
	assert.Same(t, e.full, resp.State)
	_, err = s.GetWorkflowExecution(ctx, getReq)
	assert.Same(t, errNotExists, err)
	exists, err := s.IsWorkflowExecutionExists(ctx, existsReq)
	require.NoError(t, err)
	assert.False(t, exists.Exists)
	require.NoError(t, s.UpdateWorkflowExecution(ctx, updateReq))
	require.NoError(t, s.DeleteWorkflowExecution(ctx, deleteReq))
	assert.False(t, s.inUse.Load())
}
