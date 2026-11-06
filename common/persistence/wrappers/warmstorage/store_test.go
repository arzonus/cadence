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
	"github.com/uber/cadence/common/constants"
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

// newTestStore builds the decorator with one mode for every domain.
func newTestStore(t *testing.T, mode string) (*store, *testEnv) {
	ctrl := gomock.NewController(t)
	env := &testEnv{
		inner:  persistence.NewMockExecutionStore(ctrl),
		closed: persistence.NewMockClosedExecutionStore(ctrl),
		scope:  tally.NewTestScope("", nil),
		key:    testKey(),
	}
	env.full = closedState(t, env.key)
	dc := persistence.NewDefaultDynamicConfiguration()
	dc.WarmStorageMode = func(string) string { return mode }
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

// moveDeleteRequest is the delete the move issues: conditional on the row it copied.
func (e *testEnv) moveDeleteRequest() *persistence.DeleteWorkflowExecutionRequest {
	req := e.deleteRequest()
	req.NextEventID = &e.full.ExecutionInfo.NextEventID
	return req
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

func TestGetWorkflowExecution(t *testing.T) {
	tests := map[string]struct {
		mode      string
		setup     func(t *testing.T, e *testEnv)
		wantSlim  bool
		wantErr   error
		wantFound bool
		wantHits  int64
	}{
		"open or not yet moved run comes from executions, closed table untouched": {
			mode: "on",
			setup: func(t *testing.T, e *testEnv) {
				e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), e.getRequest()).Return(&persistence.InternalGetWorkflowExecutionResponse{State: e.full}, nil)
			},
			wantFound: true,
		},
		"off: miss is returned as is, closed table untouched": {
			mode: "off",
			setup: func(t *testing.T, e *testEnv) {
				e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), e.getRequest()).Return(nil, errNotExists)
			},
			wantErr: errNotExists,
		},
		"on: Warm run is read from closed_executions": {
			mode: "on",
			setup: func(t *testing.T, e *testEnv) {
				e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), e.getRequest()).Return(nil, errNotExists)
				e.closed.EXPECT().GetClosedExecution(gomock.Any(), e.key).Return(e.slimBlob(t), nil)
			},
			wantSlim: true, wantHits: 1,
		},
		"read_only: Warm run is still read from closed_executions": {
			mode: "read_only",
			setup: func(t *testing.T, e *testEnv) {
				e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), e.getRequest()).Return(nil, errNotExists)
				e.closed.EXPECT().GetClosedExecution(gomock.Any(), e.key).Return(e.slimBlob(t), nil)
			},
			wantSlim: true, wantHits: 1,
		},
		"missing run: original not-exists error": {
			mode: "on",
			setup: func(t *testing.T, e *testEnv) {
				e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), e.getRequest()).Return(nil, errNotExists)
				e.closed.EXPECT().GetClosedExecution(gomock.Any(), e.key).Return(nil, &types.EntityNotExistsError{Message: "closed"})
			},
			wantErr: errNotExists,
		},
		"executions error other than not-exists is not masked": {
			mode: "on",
			setup: func(t *testing.T, e *testEnv) {
				e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), e.getRequest()).Return(nil, errDB)
			},
			wantErr: errDB,
		},
		"closed_executions error is returned": {
			mode: "on",
			setup: func(t *testing.T, e *testEnv) {
				e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), e.getRequest()).Return(nil, errNotExists)
				e.closed.EXPECT().GetClosedExecution(gomock.Any(), e.key).Return(nil, errDB)
			},
			wantErr: errDB,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			s, e := newTestStore(t, tc.mode)
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
		mode    string
		setup   func(e *testEnv)
		want    bool
		wantErr error
	}{
		"in executions": {
			mode: "on",
			setup: func(e *testEnv) {
				e.inner.EXPECT().IsWorkflowExecutionExists(gomock.Any(), gomock.Any()).Return(&persistence.IsWorkflowExecutionExistsResponse{Exists: true}, nil)
			},
			want: true,
		},
		"Warm run exists, so the fixer won't treat its current row as orphaned": {
			mode: "on",
			setup: func(e *testEnv) {
				e.inner.EXPECT().IsWorkflowExecutionExists(gomock.Any(), gomock.Any()).Return(&persistence.IsWorkflowExecutionExistsResponse{}, nil)
				e.closed.EXPECT().GetClosedExecution(gomock.Any(), e.key).Return(&persistence.DataBlob{}, nil)
			},
			want: true,
		},
		"missing everywhere": {
			mode: "on",
			setup: func(e *testEnv) {
				e.inner.EXPECT().IsWorkflowExecutionExists(gomock.Any(), gomock.Any()).Return(&persistence.IsWorkflowExecutionExistsResponse{}, nil)
				e.closed.EXPECT().GetClosedExecution(gomock.Any(), e.key).Return(nil, errNotExists)
			},
		},
		"off: no closed lookup": {
			mode: "off",
			setup: func(e *testEnv) {
				e.inner.EXPECT().IsWorkflowExecutionExists(gomock.Any(), gomock.Any()).Return(&persistence.IsWorkflowExecutionExistsResponse{}, nil)
			},
		},
		"store error (SQL doesn't implement it) passes through": {
			mode: "on",
			setup: func(e *testEnv) {
				e.inner.EXPECT().IsWorkflowExecutionExists(gomock.Any(), gomock.Any()).Return(nil, errDB)
			},
			wantErr: errDB,
		},
		"closed lookup error": {
			mode: "on",
			setup: func(e *testEnv) {
				e.inner.EXPECT().IsWorkflowExecutionExists(gomock.Any(), gomock.Any()).Return(&persistence.IsWorkflowExecutionExistsResponse{}, nil)
				e.closed.EXPECT().GetClosedExecution(gomock.Any(), e.key).Return(nil, errDB)
			},
			wantErr: errDB,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			s, e := newTestStore(t, tc.mode)
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
		mode    string
		setup   func(e *testEnv)
		wantErr error
	}{
		"off: only executions": {
			mode: "off",
			setup: func(e *testEnv) {
				e.inner.EXPECT().DeleteWorkflowExecution(gomock.Any(), e.deleteRequest()).Return(nil)
			},
		},
		"retention deletes both rows": {
			mode: "on",
			setup: func(e *testEnv) {
				gomock.InOrder(
					e.inner.EXPECT().DeleteWorkflowExecution(gomock.Any(), e.deleteRequest()).Return(nil),
					e.closed.EXPECT().DeleteClosedExecution(gomock.Any(), e.key).Return(nil),
				)
			},
		},
		"read_only: still deletes both": {
			mode: "read_only",
			setup: func(e *testEnv) {
				e.inner.EXPECT().DeleteWorkflowExecution(gomock.Any(), e.deleteRequest()).Return(nil)
				e.closed.EXPECT().DeleteClosedExecution(gomock.Any(), e.key).Return(nil)
			},
		},
		"executions delete fails: closed row kept for the retry": {
			mode: "on",
			setup: func(e *testEnv) {
				e.inner.EXPECT().DeleteWorkflowExecution(gomock.Any(), e.deleteRequest()).Return(errDB)
			},
			wantErr: errDB,
		},
		"closed delete fails: error so retention retries": {
			mode: "on",
			setup: func(e *testEnv) {
				e.inner.EXPECT().DeleteWorkflowExecution(gomock.Any(), e.deleteRequest()).Return(nil)
				e.closed.EXPECT().DeleteClosedExecution(gomock.Any(), e.key).Return(errDB)
			},
			wantErr: errDB,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			s, e := newTestStore(t, tc.mode)
			tc.setup(e)
			assert.Equal(t, tc.wantErr, s.DeleteWorkflowExecution(context.Background(), e.deleteRequest()))
		})
	}
}

func TestMoveToClosedExecutions(t *testing.T) {
	tests := map[string]struct {
		setup         func(t *testing.T, e *testEnv)
		wantMoves     int64
		wantFailures  int64
		wantConflicts int64
		wantSkipped   int64
	}{
		"moves: slim row, then delete": {
			setup: func(t *testing.T, e *testEnv) {
				gomock.InOrder(
					e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), e.getRequest()).Return(&persistence.InternalGetWorkflowExecutionResponse{State: e.full}, nil),
					e.closed.EXPECT().InsertClosedExecution(gomock.Any(), e.key, e.slimBlob(t)).Return(nil),
					e.inner.EXPECT().DeleteWorkflowExecution(gomock.Any(), e.moveDeleteRequest()).Return(nil),
				)
			},
			wantMoves: 1,
		},
		"close task delivered again after the move: no-op": {
			setup: func(t *testing.T, e *testEnv) {
				e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), e.getRequest()).Return(nil, errNotExists)
			},
		},
		"open run (standby hasn't closed it yet) is skipped quietly": {
			setup: func(t *testing.T, e *testEnv) {
				e.full.ExecutionInfo.State = persistence.WorkflowStateRunning
				e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), e.getRequest()).Return(&persistence.InternalGetWorkflowExecutionResponse{State: e.full}, nil)
			},
		},
		"zombie run is skipped quietly": {
			setup: func(t *testing.T, e *testEnv) {
				e.full.ExecutionInfo.State = persistence.WorkflowStateZombie
				e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), e.getRequest()).Return(&persistence.InternalGetWorkflowExecutionResponse{State: e.full}, nil)
			},
		},
		"closed with an activity still in flight stays hot: Describe lists it": {
			setup: func(t *testing.T, e *testEnv) {
				e.full.ActivityInfos = map[int64]*persistence.InternalActivityInfo{5: {ScheduleID: 5}}
				e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), e.getRequest()).Return(&persistence.InternalGetWorkflowExecutionResponse{State: e.full}, nil)
			},
			wantSkipped: 1,
		},
		"closed with a child still open stays hot: Describe lists it": {
			setup: func(t *testing.T, e *testEnv) {
				e.full.ChildExecutionInfos = map[int64]*persistence.InternalChildExecutionInfo{7: {InitiatedID: 7}}
				e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), e.getRequest()).Return(&persistence.InternalGetWorkflowExecutionResponse{State: e.full}, nil)
			},
			wantSkipped: 1,
		},
		"slim row that wouldn't read back (branch without items) is never written": {
			setup: func(t *testing.T, e *testEnv) {
				blob, err := persistence.NewPayloadSerializer().SerializeVersionHistories(&types.VersionHistories{
					Histories: []*types.VersionHistory{{BranchToken: []byte("b")}},
				}, constants.EncodingTypeThriftRW)
				require.NoError(t, err)
				e.full.VersionHistories = blob
				e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), e.getRequest()).Return(&persistence.InternalGetWorkflowExecutionResponse{State: e.full}, nil)
			},
			wantFailures: 1,
		},
		"row NextEventID disagrees with version histories: stays hot": {
			setup: func(t *testing.T, e *testEnv) {
				e.full.ExecutionInfo.NextEventID = lastEventID + 5
				e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), e.getRequest()).Return(&persistence.InternalGetWorkflowExecutionResponse{State: e.full}, nil)
			},
			wantFailures: 1,
		},
		"legacy run without version histories stays hot": {
			setup: func(t *testing.T, e *testEnv) {
				e.full.VersionHistories = nil
				e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), e.getRequest()).Return(&persistence.InternalGetWorkflowExecutionResponse{State: e.full}, nil)
			},
			wantFailures: 1,
		},
		"read fails: run stays hot, close task not failed": {
			setup: func(t *testing.T, e *testEnv) {
				e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), e.getRequest()).Return(nil, errDB)
			},
			wantFailures: 1,
		},
		"slim write fails: executions row is NOT deleted": {
			setup: func(t *testing.T, e *testEnv) {
				e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), e.getRequest()).Return(&persistence.InternalGetWorkflowExecutionResponse{State: e.full}, nil)
				e.closed.EXPECT().InsertClosedExecution(gomock.Any(), e.key, gomock.Any()).Return(errDB)
			},
			wantFailures: 1,
		},
		"a writer got in since the read: executions row is the truth, stale slim row is dropped": {
			setup: func(t *testing.T, e *testEnv) {
				gomock.InOrder(
					e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), e.getRequest()).Return(&persistence.InternalGetWorkflowExecutionResponse{State: e.full}, nil),
					e.closed.EXPECT().InsertClosedExecution(gomock.Any(), e.key, gomock.Any()).Return(nil),
					e.inner.EXPECT().DeleteWorkflowExecution(gomock.Any(), e.moveDeleteRequest()).Return(&persistence.ConditionFailedError{Msg: "next event ID changed"}),
					e.closed.EXPECT().DeleteClosedExecution(gomock.Any(), e.key).Return(nil),
				)
			},
			wantConflicts: 1,
		},
		"a writer got in and the stale slim row can't be dropped": {
			setup: func(t *testing.T, e *testEnv) {
				e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), e.getRequest()).Return(&persistence.InternalGetWorkflowExecutionResponse{State: e.full}, nil)
				e.closed.EXPECT().InsertClosedExecution(gomock.Any(), e.key, gomock.Any()).Return(nil)
				e.inner.EXPECT().DeleteWorkflowExecution(gomock.Any(), e.moveDeleteRequest()).Return(&persistence.ConditionFailedError{})
				e.closed.EXPECT().DeleteClosedExecution(gomock.Any(), e.key).Return(errDB)
			},
			wantConflicts: 1, wantFailures: 1,
		},
		"another mover finished first: the slim row it wrote is kept": {
			setup: func(t *testing.T, e *testEnv) {
				e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), e.getRequest()).Return(&persistence.InternalGetWorkflowExecutionResponse{State: e.full}, nil)
				e.closed.EXPECT().InsertClosedExecution(gomock.Any(), e.key, gomock.Any()).Return(nil)
				e.inner.EXPECT().DeleteWorkflowExecution(gomock.Any(), e.moveDeleteRequest()).Return(errNotExists)
			},
		},
		"delete fails after slim write: both rows exist, reads prefer executions": {
			setup: func(t *testing.T, e *testEnv) {
				e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), e.getRequest()).Return(&persistence.InternalGetWorkflowExecutionResponse{State: e.full}, nil)
				e.closed.EXPECT().InsertClosedExecution(gomock.Any(), e.key, gomock.Any()).Return(nil)
				e.inner.EXPECT().DeleteWorkflowExecution(gomock.Any(), e.moveDeleteRequest()).Return(errDB)
			},
			wantFailures: 1,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			s, e := newTestStore(t, "on")
			tc.setup(t, e)
			assert.NoError(t, s.MoveToClosedExecutions(context.Background(), e.deleteRequest()))
			assert.Equal(t, tc.wantMoves, e.counter("warm_storage_moves"))
			assert.Equal(t, tc.wantFailures, e.counter("warm_storage_move_failures"))
			assert.Equal(t, tc.wantConflicts, e.counter("warm_storage_move_conflicts"))
			assert.Equal(t, tc.wantSkipped, e.counter("warm_storage_moves_skipped"))
		})
	}
}

// TestModeMatrix pins, for every mode and operation, exactly which closed-store calls happen. The gomock closed
// store is strict, so in off any call at all fails the test: that is the proof of zero reads.
func TestModeMatrix(t *testing.T) {
	notThere := &types.EntityNotExistsError{Message: "update on a missing row"}
	hotResp := func(e *testEnv) *persistence.InternalGetWorkflowExecutionResponse {
		return &persistence.InternalGetWorkflowExecutionResponse{State: e.full}
	}
	// differs is a slim row that disagrees with the hot row on one field
	differs := func(t *testing.T, e *testEnv) *persistence.DataBlob {
		other := *e.full
		info := *other.ExecutionInfo
		info.TaskList = "other-tl"
		other.ExecutionInfo = &info
		blob, err := toBlob(testParser(t), &other)
		require.NoError(t, err)
		return blob
	}
	type op struct {
		// expect sets the calls the wrapped and closed stores must see in the given mode
		expect func(t *testing.T, e *testEnv, mode string)
		// run calls the decorator and checks the result
		run func(t *testing.T, s *store, e *testEnv, mode string)
		// mismatches is the shadow mismatch counter expected, by mode
		mismatches int64
	}
	getHit := func(t *testing.T, s *store, e *testEnv, mode string) {
		resp, err := s.GetWorkflowExecution(context.Background(), e.getRequest())
		require.NoError(t, err)
		assert.Same(t, e.full, resp.State, "the hot row is returned as is")
	}
	// readsSlimOnHit is where the shadow compare reads the slim row of a hot closed run
	readsSlimOnHit := func(mode string) bool { return mode == "shadow" }

	ops := map[string]op{
		"get hit, no slim row": {
			expect: func(t *testing.T, e *testEnv, mode string) {
				e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), e.getRequest()).Return(hotResp(e), nil)
				if readsSlimOnHit(mode) {
					e.closed.EXPECT().GetClosedExecution(gomock.Any(), e.key).Return(nil, errNotExists)
				}
			},
			run: getHit,
		},
		"get hit, slim row matches": {
			expect: func(t *testing.T, e *testEnv, mode string) {
				e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), e.getRequest()).Return(hotResp(e), nil)
				if readsSlimOnHit(mode) {
					e.closed.EXPECT().GetClosedExecution(gomock.Any(), e.key).Return(e.slimBlob(t), nil)
				}
			},
			run: getHit,
		},
		"get hit, slim row differs": {
			expect: func(t *testing.T, e *testEnv, mode string) {
				e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), e.getRequest()).Return(hotResp(e), nil)
				if readsSlimOnHit(mode) {
					e.closed.EXPECT().GetClosedExecution(gomock.Any(), e.key).Return(differs(t, e), nil)
				}
			},
			run:        getHit,
			mismatches: 1,
		},
		"get miss, slim row": {
			expect: func(t *testing.T, e *testEnv, mode string) {
				e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), e.getRequest()).Return(nil, errNotExists)
				if mode != "off" {
					e.closed.EXPECT().GetClosedExecution(gomock.Any(), e.key).Return(e.slimBlob(t), nil)
				}
			},
			run: func(t *testing.T, s *store, e *testEnv, mode string) {
				resp, err := s.GetWorkflowExecution(context.Background(), e.getRequest())
				if mode == "off" {
					assert.Equal(t, errNotExists, err)
					return
				}
				require.NoError(t, err)
				assert.Equal(t, expectedSlimState(t, e.full), resp.State)
			},
		},
		"get miss, no slim row": {
			expect: func(t *testing.T, e *testEnv, mode string) {
				e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), e.getRequest()).Return(nil, errNotExists)
				if mode != "off" {
					e.closed.EXPECT().GetClosedExecution(gomock.Any(), e.key).Return(nil, errNotExists)
				}
			},
			run: func(t *testing.T, s *store, e *testEnv, mode string) {
				_, err := s.GetWorkflowExecution(context.Background(), e.getRequest())
				assert.Equal(t, errNotExists, err)
			},
		},
		"is exists, only in the slim table": {
			expect: func(t *testing.T, e *testEnv, mode string) {
				e.inner.EXPECT().IsWorkflowExecutionExists(gomock.Any(), gomock.Any()).Return(&persistence.IsWorkflowExecutionExistsResponse{}, nil)
				if mode != "off" {
					e.closed.EXPECT().GetClosedExecution(gomock.Any(), e.key).Return(&persistence.DataBlob{}, nil)
				}
			},
			run: func(t *testing.T, s *store, e *testEnv, mode string) {
				resp, err := s.IsWorkflowExecutionExists(context.Background(), &persistence.IsWorkflowExecutionExistsRequest{
					DomainID: e.key.DomainID, WorkflowID: e.key.WorkflowID, RunID: e.key.RunID,
				})
				require.NoError(t, err)
				assert.Equal(t, mode != "off", resp.Exists)
			},
		},
		"delete": {
			expect: func(t *testing.T, e *testEnv, mode string) {
				e.inner.EXPECT().DeleteWorkflowExecution(gomock.Any(), e.deleteRequest()).Return(nil)
				if mode != "off" {
					e.closed.EXPECT().DeleteClosedExecution(gomock.Any(), e.key).Return(nil)
				}
			},
			run: func(t *testing.T, s *store, e *testEnv, mode string) {
				assert.NoError(t, s.DeleteWorkflowExecution(context.Background(), e.deleteRequest()))
			},
		},
		"update that needs a put-back": {
			expect: func(t *testing.T, e *testEnv, mode string) {
				req := putBackRequest(e)
				if mode == "off" {
					e.inner.EXPECT().UpdateWorkflowExecution(gomock.Any(), req).Return(notThere)
					return
				}
				gomock.InOrder(
					e.inner.EXPECT().UpdateWorkflowExecution(gomock.Any(), req).Return(notThere),
					e.closed.EXPECT().GetClosedExecution(gomock.Any(), e.key).Return(e.slimBlob(t), nil),
					e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), gomock.Any()).Return(nil, errNotExists),
					e.inner.EXPECT().CreateWorkflowExecution(gomock.Any(), gomock.Any()).Return(&persistence.CreateWorkflowExecutionResponse{}, nil),
					e.inner.EXPECT().UpdateWorkflowExecution(gomock.Any(), req).Return(nil),
				)
			},
			run: func(t *testing.T, s *store, e *testEnv, mode string) {
				err := s.UpdateWorkflowExecution(context.Background(), putBackRequest(e))
				if mode == "off" {
					assert.Equal(t, notThere, err)
					return
				}
				assert.NoError(t, err)
			},
		},
		"move": {
			expect: func(t *testing.T, e *testEnv, mode string) {
				switch mode {
				case "shadow":
					e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), e.getRequest()).Return(hotResp(e), nil)
					e.closed.EXPECT().InsertClosedExecution(gomock.Any(), e.key, e.slimBlob(t)).Return(nil)
				case "on":
					e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), e.getRequest()).Return(hotResp(e), nil)
					e.closed.EXPECT().InsertClosedExecution(gomock.Any(), e.key, e.slimBlob(t)).Return(nil)
					e.inner.EXPECT().DeleteWorkflowExecution(gomock.Any(), e.moveDeleteRequest()).Return(nil)
				}
			},
			run: func(t *testing.T, s *store, e *testEnv, mode string) {
				assert.NoError(t, s.MoveToClosedExecutions(context.Background(), e.deleteRequest()))
				wantMoves := int64(0)
				if mode == "on" {
					wantMoves = 1
				}
				assert.Equal(t, wantMoves, e.counter("warm_storage_moves"))
			},
		},
	}
	for opName, o := range ops {
		for _, mode := range []string{"off", "shadow", "read_only", "on"} {
			t.Run(mode+"/"+opName, func(t *testing.T) {
				s, e := newTestStore(t, mode)
				o.expect(t, e, mode)
				o.run(t, s, e, mode)
				want := int64(0)
				if mode == "shadow" {
					want = o.mismatches
				}
				assert.Equal(t, want, e.counter("warm_storage_shadow_mismatches"))
			})
		}
	}
}

func putBackRequest(e *testEnv) *persistence.InternalUpdateWorkflowExecutionRequest {
	return &persistence.InternalUpdateWorkflowExecutionRequest{
		ShardID: common.IntPtr(7),
		RangeID: 9,
		UpdateWorkflowMutation: persistence.InternalWorkflowMutation{ExecutionInfo: &persistence.InternalWorkflowExecutionInfo{
			DomainID: e.key.DomainID, WorkflowID: e.key.WorkflowID, RunID: e.key.RunID,
		}},
	}
}

// Shadow follows the move's skip rules and fails open like on, and never deletes the executions row.
func TestShadowMove(t *testing.T) {
	tests := map[string]struct {
		setup        func(t *testing.T, e *testEnv)
		wantFailures int64
		wantSkipped  int64
	}{
		"open run is skipped": {
			setup: func(t *testing.T, e *testEnv) {
				e.full.ExecutionInfo.State = persistence.WorkflowStateRunning
				e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), e.getRequest()).Return(&persistence.InternalGetWorkflowExecutionResponse{State: e.full}, nil)
			},
		},
		"activity in flight is skipped and counted": {
			setup: func(t *testing.T, e *testEnv) {
				e.full.ActivityInfos = map[int64]*persistence.InternalActivityInfo{5: {ScheduleID: 5}}
				e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), e.getRequest()).Return(&persistence.InternalGetWorkflowExecutionResponse{State: e.full}, nil)
			},
			wantSkipped: 1,
		},
		"child in flight is skipped and counted": {
			setup: func(t *testing.T, e *testEnv) {
				e.full.ChildExecutionInfos = map[int64]*persistence.InternalChildExecutionInfo{7: {InitiatedID: 7}}
				e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), e.getRequest()).Return(&persistence.InternalGetWorkflowExecutionResponse{State: e.full}, nil)
			},
			wantSkipped: 1,
		},
		"slim write fails: fails open": {
			setup: func(t *testing.T, e *testEnv) {
				e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), e.getRequest()).Return(&persistence.InternalGetWorkflowExecutionResponse{State: e.full}, nil)
				e.closed.EXPECT().InsertClosedExecution(gomock.Any(), e.key, gomock.Any()).Return(errDB)
			},
			wantFailures: 1,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			s, e := newTestStore(t, "shadow")
			tc.setup(t, e)
			assert.NoError(t, s.MoveToClosedExecutions(context.Background(), e.deleteRequest()))
			assert.Equal(t, tc.wantFailures, e.counter("warm_storage_move_failures"))
			assert.Equal(t, tc.wantSkipped, e.counter("warm_storage_moves_skipped"))
			assert.Zero(t, e.counter("warm_storage_moves"))
		})
	}
}

// A shadow compare that can't read or decode the slim row logs and still returns the hot row.
func TestShadowCompareErrorsOnlyLog(t *testing.T) {
	for name, closedResp := range map[string]struct {
		blob *persistence.DataBlob
		err  error
	}{
		"read error":   {nil, errDB},
		"decode error": {persistence.NewDataBlob([]byte("garbage"), constants.EncodingTypeThriftRW), nil},
	} {
		t.Run(name, func(t *testing.T) {
			s, e := newTestStore(t, "shadow")
			e.inner.EXPECT().GetWorkflowExecution(gomock.Any(), e.getRequest()).Return(&persistence.InternalGetWorkflowExecutionResponse{State: e.full}, nil)
			e.closed.EXPECT().GetClosedExecution(gomock.Any(), e.key).Return(closedResp.blob, closedResp.err)
			resp, err := s.GetWorkflowExecution(context.Background(), e.getRequest())
			require.NoError(t, err)
			assert.Same(t, e.full, resp.State)
			assert.Zero(t, e.counter("warm_storage_shadow_mismatches"))
		})
	}
}
