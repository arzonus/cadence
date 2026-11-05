package postgres

import (
	"context"
	"database/sql"

	"github.com/uber/cadence/common/persistence/sql/sqlplugin"
)

const (
	upsertClosedExecutionQuery = `INSERT INTO closed_executions (domain_id, workflow_id, run_id, data, data_encoding)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (domain_id, workflow_id, run_id) DO UPDATE
		SET data = excluded.data, data_encoding = excluded.data_encoding`

	getClosedExecutionQuery = `SELECT domain_id, workflow_id, run_id, data, data_encoding
		FROM closed_executions WHERE domain_id = $1 AND workflow_id = $2 AND run_id = $3`

	deleteClosedExecutionQuery = `DELETE FROM closed_executions
		WHERE domain_id = $1 AND workflow_id = $2 AND run_id = $3`
)

// closed_executions is keyed by run, so it is routed by domain rather than history shard.
func (pdb *db) UpsertIntoClosedExecutions(ctx context.Context, row *sqlplugin.ClosedExecutionRow) (sql.Result, error) {
	dbShardID := sqlplugin.GetDBShardIDFromDomainID(row.DomainID.String(), pdb.GetTotalNumDBShards())
	return pdb.driver.ExecContext(ctx, dbShardID, upsertClosedExecutionQuery,
		row.DomainID, row.WorkflowID, row.RunID, row.Data, row.DataEncoding)
}

func (pdb *db) SelectFromClosedExecutions(ctx context.Context, filter *sqlplugin.ClosedExecutionFilter) (*sqlplugin.ClosedExecutionRow, error) {
	dbShardID := sqlplugin.GetDBShardIDFromDomainID(filter.DomainID.String(), pdb.GetTotalNumDBShards())
	var row sqlplugin.ClosedExecutionRow
	if err := pdb.driver.GetContext(ctx, dbShardID, &row, getClosedExecutionQuery,
		filter.DomainID, filter.WorkflowID, filter.RunID); err != nil {
		return nil, err
	}
	return &row, nil
}

func (pdb *db) DeleteFromClosedExecutions(ctx context.Context, filter *sqlplugin.ClosedExecutionFilter) (sql.Result, error) {
	dbShardID := sqlplugin.GetDBShardIDFromDomainID(filter.DomainID.String(), pdb.GetTotalNumDBShards())
	return pdb.driver.ExecContext(ctx, dbShardID, deleteClosedExecutionQuery,
		filter.DomainID, filter.WorkflowID, filter.RunID)
}
