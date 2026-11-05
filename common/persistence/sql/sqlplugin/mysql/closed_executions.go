package mysql

import (
	"context"
	"database/sql"

	"github.com/uber/cadence/common/persistence/sql/sqlplugin"
)

const (
	upsertClosedExecutionQuery = `REPLACE INTO closed_executions
		(domain_id, workflow_id, run_id, data, data_encoding) VALUES (?, ?, ?, ?, ?)`

	getClosedExecutionQuery = `SELECT domain_id, workflow_id, run_id, data, data_encoding
		FROM closed_executions WHERE domain_id = ? AND workflow_id = ? AND run_id = ?`

	deleteClosedExecutionQuery = `DELETE FROM closed_executions
		WHERE domain_id = ? AND workflow_id = ? AND run_id = ?`
)

// closed_executions is keyed by run, so it is routed by domain rather than history shard.
func (mdb *DB) UpsertIntoClosedExecutions(ctx context.Context, row *sqlplugin.ClosedExecutionRow) (sql.Result, error) {
	dbShardID := sqlplugin.GetDBShardIDFromDomainID(row.DomainID.String(), mdb.GetTotalNumDBShards())
	return mdb.driver.ExecContext(ctx, dbShardID, upsertClosedExecutionQuery,
		row.DomainID, row.WorkflowID, row.RunID, row.Data, row.DataEncoding)
}

func (mdb *DB) SelectFromClosedExecutions(ctx context.Context, filter *sqlplugin.ClosedExecutionFilter) (*sqlplugin.ClosedExecutionRow, error) {
	dbShardID := sqlplugin.GetDBShardIDFromDomainID(filter.DomainID.String(), mdb.GetTotalNumDBShards())
	var row sqlplugin.ClosedExecutionRow
	if err := mdb.driver.GetContext(ctx, dbShardID, &row, getClosedExecutionQuery,
		filter.DomainID, filter.WorkflowID, filter.RunID); err != nil {
		return nil, err
	}
	return &row, nil
}

func (mdb *DB) DeleteFromClosedExecutions(ctx context.Context, filter *sqlplugin.ClosedExecutionFilter) (sql.Result, error) {
	dbShardID := sqlplugin.GetDBShardIDFromDomainID(filter.DomainID.String(), mdb.GetTotalNumDBShards())
	return mdb.driver.ExecContext(ctx, dbShardID, deleteClosedExecutionQuery,
		filter.DomainID, filter.WorkflowID, filter.RunID)
}
