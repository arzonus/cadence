package persistencetests

import (
	"testing"

	"github.com/pborman/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/uber/cadence/common/constants"
	"github.com/uber/cadence/common/persistence"
	"github.com/uber/cadence/common/types"
)

// CheckClosedExecutionStore runs the closed_executions round trip against the given store.
func CheckClosedExecutionStore(t *testing.T, store persistence.ClosedExecutionStore) {
	ctx := t.Context()
	key := persistence.ClosedExecutionKey{DomainID: uuid.New(), WorkflowID: "closed-wf", RunID: uuid.New()}

	_, err := store.GetClosedExecution(ctx, key)
	assert.IsType(t, &types.EntityNotExistsError{}, err, "missing row")
	assert.NoError(t, store.DeleteClosedExecution(ctx, key), "delete of missing row")

	require.NoError(t, store.InsertClosedExecution(ctx, key, persistence.NewDataBlob([]byte("one"), constants.EncodingTypeThriftRW)))
	got, err := store.GetClosedExecution(ctx, key)
	require.NoError(t, err)
	assert.Equal(t, []byte("one"), got.Data)
	assert.Equal(t, constants.EncodingTypeThriftRW, got.Encoding)

	require.NoError(t, store.InsertClosedExecution(ctx, key, persistence.NewDataBlob([]byte("two"), constants.EncodingTypeThriftRWSnappy)))
	got, err = store.GetClosedExecution(ctx, key)
	require.NoError(t, err)
	assert.Equal(t, []byte("two"), got.Data, "upsert replaces")
	assert.Equal(t, constants.EncodingTypeThriftRWSnappy, got.Encoding)

	// A different run of the same workflow is a different row.
	other := key
	other.RunID = uuid.New()
	_, err = store.GetClosedExecution(ctx, other)
	assert.IsType(t, &types.EntityNotExistsError{}, err)

	require.NoError(t, store.DeleteClosedExecution(ctx, key))
	_, err = store.GetClosedExecution(ctx, key)
	assert.IsType(t, &types.EntityNotExistsError{}, err, "deleted row")
}
