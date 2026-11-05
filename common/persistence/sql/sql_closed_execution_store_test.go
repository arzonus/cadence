package sql

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/pborman/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/uber/cadence/common/constants"
	"github.com/uber/cadence/common/log"
	"github.com/uber/cadence/common/persistence"
	"github.com/uber/cadence/common/persistence/sql/sqlplugin"
	"github.com/uber/cadence/common/types"
)

func TestSQLClosedExecutionStore(t *testing.T) {
	key := persistence.ClosedExecutionKey{DomainID: uuid.New(), WorkflowID: "wf", RunID: uuid.New()}
	blob := persistence.NewDataBlob([]byte("x"), constants.EncodingTypeThriftRW)
	boom := errors.New("boom")

	tests := []struct {
		name    string
		setup   func(db *sqlplugin.MockDB)
		call    func(s persistence.ClosedExecutionStore) error
		wantErr func(error) bool
	}{
		{
			name: "insert upserts the row",
			setup: func(db *sqlplugin.MockDB) {
				db.EXPECT().UpsertIntoClosedExecutions(gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, row *sqlplugin.ClosedExecutionRow) (sql.Result, error) {
						assert.Equal(t, key.DomainID, row.DomainID.String())
						assert.Equal(t, "wf", row.WorkflowID)
						assert.Equal(t, []byte("x"), row.Data)
						assert.Equal(t, "thriftrw", row.DataEncoding)
						return nil, nil
					})
			},
			call: func(s persistence.ClosedExecutionStore) error { return s.InsertClosedExecution(t.Context(), key, blob) },
		},
		{
			name: "insert error is converted",
			setup: func(db *sqlplugin.MockDB) {
				db.EXPECT().UpsertIntoClosedExecutions(gomock.Any(), gomock.Any()).Return(nil, boom)
				db.EXPECT().IsNotFoundError(boom).Return(false)
				db.EXPECT().IsTimeoutError(boom).Return(false)
				db.EXPECT().IsThrottlingError(boom).Return(false)
			},
			call:    func(s persistence.ClosedExecutionStore) error { return s.InsertClosedExecution(t.Context(), key, blob) },
			wantErr: func(err error) bool { _, ok := err.(*types.InternalServiceError); return ok },
		},
		{
			name: "get returns the blob",
			setup: func(db *sqlplugin.MockDB) {
				db.EXPECT().SelectFromClosedExecutions(gomock.Any(), gomock.Any()).Return(
					&sqlplugin.ClosedExecutionRow{Data: []byte("x"), DataEncoding: "thriftrw"}, nil)
			},
			call: func(s persistence.ClosedExecutionStore) error {
				got, err := s.GetClosedExecution(t.Context(), key)
				assert.Equal(t, blob, got)
				return err
			},
		},
		{
			name: "get of missing row is EntityNotExists",
			setup: func(db *sqlplugin.MockDB) {
				db.EXPECT().SelectFromClosedExecutions(gomock.Any(), gomock.Any()).Return(nil, sql.ErrNoRows)
			},
			call: func(s persistence.ClosedExecutionStore) error {
				_, err := s.GetClosedExecution(t.Context(), key)
				return err
			},
			wantErr: func(err error) bool { _, ok := err.(*types.EntityNotExistsError); return ok },
		},
		{
			name: "delete succeeds even when nothing was deleted",
			setup: func(db *sqlplugin.MockDB) {
				db.EXPECT().DeleteFromClosedExecutions(gomock.Any(), gomock.Any()).Return(nil, nil)
			},
			call: func(s persistence.ClosedExecutionStore) error { return s.DeleteClosedExecution(t.Context(), key) },
		},
		{
			name: "delete error is converted",
			setup: func(db *sqlplugin.MockDB) {
				db.EXPECT().DeleteFromClosedExecutions(gomock.Any(), gomock.Any()).Return(nil, boom)
				db.EXPECT().IsNotFoundError(boom).Return(false)
				db.EXPECT().IsTimeoutError(boom).Return(false)
				db.EXPECT().IsThrottlingError(boom).Return(false)
			},
			call:    func(s persistence.ClosedExecutionStore) error { return s.DeleteClosedExecution(t.Context(), key) },
			wantErr: func(err error) bool { _, ok := err.(*types.InternalServiceError); return ok },
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			db := sqlplugin.NewMockDB(ctrl)
			tc.setup(db)
			err := tc.call(newSQLClosedExecutionStore(db, log.NewNoop(), nil))
			if tc.wantErr == nil {
				assert.NoError(t, err)
				return
			}
			assert.True(t, tc.wantErr(err), "unexpected error: %v", err)
		})
	}
}
