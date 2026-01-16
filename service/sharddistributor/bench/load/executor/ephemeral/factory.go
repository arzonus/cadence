package ephemeral

import (
	"time"

	"github.com/uber/cadence/common/clock"
	"github.com/uber/cadence/common/log"
	"github.com/uber/cadence/service/sharddistributor/client/executorclient"
)

// Factory implements ShardProcessorFactory for ephemeral shards
type Factory struct {
	shardLoadFn   func(shardID string) float64
	shardLifetime time.Duration
	timeSource    clock.TimeSource
	logger        log.Logger
}

// NewFactory creates a new ephemeral shard processor factory
func NewFactory(
	shardLoadFn func(shardID string) float64,
	shardLifetime time.Duration,
	timeSource clock.TimeSource,
	logger log.Logger,
) *Factory {
	return &Factory{
		shardLoadFn:   shardLoadFn,
		shardLifetime: shardLifetime,
		timeSource:    timeSource,
		logger:        logger,
	}
}

var _ executorclient.ShardProcessorFactory[*ShardProcessor] = (*Factory)(nil)

// NewShardProcessor creates a new ephemeral shard processor
func (f *Factory) NewShardProcessor(shardID string) (*ShardProcessor, error) {
	return NewShardProcessor(
		shardID,
		f.timeSource,
		f.logger,
		f.shardLoadFn,
		f.shardLifetime,
	), nil
}
