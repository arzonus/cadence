package nosql

import (
	"context"

	"github.com/uber/cadence/common/config"
	"github.com/uber/cadence/common/constants"
	"github.com/uber/cadence/common/log"
	"github.com/uber/cadence/common/metrics"
	"github.com/uber/cadence/common/persistence"
	"github.com/uber/cadence/common/persistence/nosql/nosqlplugin"
)

type nosqlClosedExecutionStore struct {
	nosqlStore
}

// newNoSQLClosedExecutionStore keeps all rows on the default shard: the table is keyed by run, not by history shard.
func newNoSQLClosedExecutionStore(
	cfg config.ShardedNoSQL,
	logger log.Logger,
	metricsClient metrics.Client,
	dc *persistence.DynamicConfiguration,
) (persistence.ClosedExecutionStore, error) {
	shardedStore, err := newShardedNosqlStore(cfg, logger, metricsClient, dc, false)
	if err != nil {
		return nil, err
	}
	return &nosqlClosedExecutionStore{nosqlStore: shardedStore.GetDefaultShard()}, nil
}

func (s *nosqlClosedExecutionStore) InsertClosedExecution(ctx context.Context, key persistence.ClosedExecutionKey, data *persistence.DataBlob) error {
	err := s.db.InsertClosedExecution(ctx, &nosqlplugin.ClosedExecutionRow{
		DomainID:     key.DomainID,
		WorkflowID:   key.WorkflowID,
		RunID:        key.RunID,
		Data:         getDataBlobBytes(data),
		DataEncoding: getDataBlobEncoding(data),
	})
	if err != nil {
		return convertCommonErrors(s.db, "InsertClosedExecution", err)
	}
	return nil
}

func (s *nosqlClosedExecutionStore) GetClosedExecution(ctx context.Context, key persistence.ClosedExecutionKey) (*persistence.DataBlob, error) {
	row, err := s.db.SelectClosedExecution(ctx, key.DomainID, key.WorkflowID, key.RunID)
	if err != nil {
		return nil, convertCommonErrors(s.db, "GetClosedExecution", err)
	}
	return &persistence.DataBlob{Data: row.Data, Encoding: constants.EncodingType(row.DataEncoding)}, nil
}

func (s *nosqlClosedExecutionStore) DeleteClosedExecution(ctx context.Context, key persistence.ClosedExecutionKey) error {
	if err := s.db.DeleteClosedExecution(ctx, key.DomainID, key.WorkflowID, key.RunID); err != nil {
		return convertCommonErrors(s.db, "DeleteClosedExecution", err)
	}
	return nil
}
