package warmstorage

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/uber/cadence/common/persistence"
)

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
