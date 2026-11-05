package mysql

import (
	"testing"

	"github.com/stretchr/testify/require"

	pt "github.com/uber/cadence/common/persistence/persistence-tests"
	"github.com/uber/cadence/common/persistence/sql"
)

func TestMySQLClosedExecutionStore(t *testing.T) {
	b := MySQLTestBase(t)
	cfg := b.PersistenceConfig
	store, err := sql.NewFactory(*cfg.DataStores[cfg.DefaultStore].SQL, "", b.Logger, nil, &b.DynamicConfiguration).NewClosedExecutionStore()
	require.NoError(t, err)
	pt.CheckClosedExecutionStore(t, store)
}
