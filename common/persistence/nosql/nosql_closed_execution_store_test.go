package nosql

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/uber/cadence/common/constants"
	"github.com/uber/cadence/common/persistence"
	"github.com/uber/cadence/common/persistence/nosql/nosqlplugin"
	"github.com/uber/cadence/common/types"
)

func TestClosedExecutionStore(t *testing.T) {
	key := persistence.ClosedExecutionKey{DomainID: "d", WorkflowID: "w", RunID: "r"}
	blob := &persistence.DataBlob{Data: []byte("slim"), Encoding: constants.EncodingTypeThriftRWSnappy}
	row := &nosqlplugin.ClosedExecutionRow{
		DomainID: "d", WorkflowID: "w", RunID: "r",
		Data: []byte("slim"), DataEncoding: string(constants.EncodingTypeThriftRWSnappy),
	}
	boom := errors.New("boom")

	tests := []struct {
		name    string
		op      func(context.Context, persistence.ClosedExecutionStore) (*persistence.DataBlob, error)
		setup   func(*nosqlplugin.MockDB)
		want    *persistence.DataBlob
		wantErr error
	}{
		{
			name: "insert",
			op: func(ctx context.Context, s persistence.ClosedExecutionStore) (*persistence.DataBlob, error) {
				return nil, s.InsertClosedExecution(ctx, key, blob)
			},
			setup: func(db *nosqlplugin.MockDB) {
				db.EXPECT().InsertClosedExecution(gomock.Any(), row).Return(nil)
			},
		},
		{
			name: "insert error",
			op: func(ctx context.Context, s persistence.ClosedExecutionStore) (*persistence.DataBlob, error) {
				return nil, s.InsertClosedExecution(ctx, key, blob)
			},
			setup: func(db *nosqlplugin.MockDB) {
				db.EXPECT().InsertClosedExecution(gomock.Any(), row).Return(boom)
				db.EXPECT().IsNotFoundError(boom).Return(false)
				db.EXPECT().IsTimeoutError(boom).Return(false)
				db.EXPECT().IsThrottlingError(boom).Return(false)
				db.EXPECT().IsDBUnavailableError(boom).Return(false)
			},
			wantErr: &types.InternalServiceError{},
		},
		{
			name: "get",
			op: func(ctx context.Context, s persistence.ClosedExecutionStore) (*persistence.DataBlob, error) {
				return s.GetClosedExecution(ctx, key)
			},
			setup: func(db *nosqlplugin.MockDB) {
				db.EXPECT().SelectClosedExecution(gomock.Any(), "d", "w", "r").Return(row, nil)
			},
			want: blob,
		},
		{
			name: "get missing",
			op: func(ctx context.Context, s persistence.ClosedExecutionStore) (*persistence.DataBlob, error) {
				return s.GetClosedExecution(ctx, key)
			},
			setup: func(db *nosqlplugin.MockDB) {
				db.EXPECT().SelectClosedExecution(gomock.Any(), "d", "w", "r").Return(nil, boom)
				db.EXPECT().IsNotFoundError(boom).Return(true)
			},
			wantErr: &types.EntityNotExistsError{},
		},
		{
			name: "delete",
			op: func(ctx context.Context, s persistence.ClosedExecutionStore) (*persistence.DataBlob, error) {
				return nil, s.DeleteClosedExecution(ctx, key)
			},
			setup: func(db *nosqlplugin.MockDB) {
				db.EXPECT().DeleteClosedExecution(gomock.Any(), "d", "w", "r").Return(nil)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db := nosqlplugin.NewMockDB(gomock.NewController(t))
			tc.setup(db)
			store := &nosqlClosedExecutionStore{nosqlStore: nosqlStore{db: db}}

			got, err := tc.op(context.Background(), store)

			if tc.wantErr != nil {
				assert.IsType(t, tc.wantErr, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}
