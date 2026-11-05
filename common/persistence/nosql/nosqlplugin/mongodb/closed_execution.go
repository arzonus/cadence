package mongodb

import (
	"context"
	"fmt"

	"github.com/uber/cadence/common/persistence/nosql/nosqlplugin"
)

func (db *mdb) InsertClosedExecution(ctx context.Context, row *nosqlplugin.ClosedExecutionRow) error {
	return fmt.Errorf("InsertClosedExecution is not implemented")
}

func (db *mdb) SelectClosedExecution(ctx context.Context, domainID, workflowID, runID string) (*nosqlplugin.ClosedExecutionRow, error) {
	return nil, fmt.Errorf("SelectClosedExecution is not implemented")
}

func (db *mdb) DeleteClosedExecution(ctx context.Context, domainID, workflowID, runID string) error {
	return fmt.Errorf("DeleteClosedExecution is not implemented")
}
