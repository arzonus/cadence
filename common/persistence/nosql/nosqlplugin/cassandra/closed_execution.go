package cassandra

import (
	"context"

	"github.com/uber/cadence/common/persistence/nosql/nosqlplugin"
)

const (
	templateInsertClosedExecutionQuery = `INSERT INTO closed_executions (` +
		`domain_id, workflow_id, run_id, data, data_encoding) ` +
		`VALUES(?, ?, ?, ?, ?)`

	templateSelectClosedExecutionQuery = `SELECT data, data_encoding ` +
		`FROM closed_executions ` +
		`WHERE domain_id = ? AND workflow_id = ? AND run_id = ?`

	templateDeleteClosedExecutionQuery = `DELETE FROM closed_executions ` +
		`WHERE domain_id = ? AND workflow_id = ? AND run_id = ?`
)

// InsertClosedExecution upserts a closed run's slim row.
func (db *CDB) InsertClosedExecution(ctx context.Context, row *nosqlplugin.ClosedExecutionRow) error {
	return db.session.Query(templateInsertClosedExecutionQuery,
		row.DomainID, row.WorkflowID, row.RunID, row.Data, row.DataEncoding,
	).WithContext(ctx).Exec()
}

// SelectClosedExecution returns a closed run's slim row. A missing row surfaces as the
// driver's not-found error, which IsNotFoundError recognizes.
func (db *CDB) SelectClosedExecution(ctx context.Context, domainID, workflowID, runID string) (*nosqlplugin.ClosedExecutionRow, error) {
	row := &nosqlplugin.ClosedExecutionRow{DomainID: domainID, WorkflowID: workflowID, RunID: runID}
	err := db.session.Query(templateSelectClosedExecutionQuery, domainID, workflowID, runID).
		WithContext(ctx).Scan(&row.Data, &row.DataEncoding)
	if err != nil {
		return nil, err
	}
	return row, nil
}

// DeleteClosedExecution deletes a closed run's slim row; deleting a missing row is a no-op.
func (db *CDB) DeleteClosedExecution(ctx context.Context, domainID, workflowID, runID string) error {
	return db.session.Query(templateDeleteClosedExecutionQuery, domainID, workflowID, runID).
		WithContext(ctx).Exec()
}
