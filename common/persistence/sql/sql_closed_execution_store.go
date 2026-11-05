package sql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/uber/cadence/common/constants"
	"github.com/uber/cadence/common/log"
	"github.com/uber/cadence/common/persistence"
	"github.com/uber/cadence/common/persistence/serialization"
	"github.com/uber/cadence/common/persistence/sql/sqlplugin"
	"github.com/uber/cadence/common/types"
)

type sqlClosedExecutionStore struct {
	sqlStore
}

func newSQLClosedExecutionStore(db sqlplugin.DB, logger log.Logger, parser serialization.Parser) persistence.ClosedExecutionStore {
	return &sqlClosedExecutionStore{sqlStore{db: db, logger: logger, parser: parser}}
}

func (m *sqlClosedExecutionStore) InsertClosedExecution(ctx context.Context, key persistence.ClosedExecutionKey, data *persistence.DataBlob) error {
	_, err := m.db.UpsertIntoClosedExecutions(ctx, &sqlplugin.ClosedExecutionRow{
		DomainID:     serialization.MustParseUUID(key.DomainID),
		WorkflowID:   key.WorkflowID,
		RunID:        serialization.MustParseUUID(key.RunID),
		Data:         getDataBlobBytes(data),
		DataEncoding: string(getDataBlobEncoding(data)),
	})
	if err != nil {
		return convertCommonErrors(m.db, "InsertClosedExecution", "", err)
	}
	return nil
}

func (m *sqlClosedExecutionStore) GetClosedExecution(ctx context.Context, key persistence.ClosedExecutionKey) (*persistence.DataBlob, error) {
	row, err := m.db.SelectFromClosedExecutions(ctx, filterOf(key))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, &types.EntityNotExistsError{
				Message: fmt.Sprintf("Closed execution not found. DomainId: %v, WorkflowId: %v, RunId: %v",
					key.DomainID, key.WorkflowID, key.RunID),
			}
		}
		return nil, convertCommonErrors(m.db, "GetClosedExecution", "", err)
	}
	return persistence.NewDataBlob(row.Data, constants.EncodingType(row.DataEncoding)), nil
}

func (m *sqlClosedExecutionStore) DeleteClosedExecution(ctx context.Context, key persistence.ClosedExecutionKey) error {
	if _, err := m.db.DeleteFromClosedExecutions(ctx, filterOf(key)); err != nil {
		return convertCommonErrors(m.db, "DeleteClosedExecution", "", err)
	}
	return nil
}

func filterOf(key persistence.ClosedExecutionKey) *sqlplugin.ClosedExecutionFilter {
	return &sqlplugin.ClosedExecutionFilter{
		DomainID:   serialization.MustParseUUID(key.DomainID),
		WorkflowID: key.WorkflowID,
		RunID:      serialization.MustParseUUID(key.RunID),
	}
}
