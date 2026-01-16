package fixed

import (
	"github.com/uber/cadence/common/clock"
	"github.com/uber/cadence/common/log"
	"github.com/uber/cadence/service/sharddistributor/client/executorclient"
)

// Factory implements ShardProcessorFactory for fixed shards
type Factory struct {
	shardLoadFn func(shardID string) float64
	timeSource  clock.TimeSource
	logger      log.Logger
}

// NewFactory creates a new fixed shard processor factory
func NewFactory(
	shardLoadFn func(shardID string) float64,
	timeSource clock.TimeSource,
	logger log.Logger,
) *Factory {
	return &Factory{
		shardLoadFn: shardLoadFn,
		timeSource:  timeSource,
		logger:      logger,
	}
}

var _ executorclient.ShardProcessorFactory[*ShardProcessor] = (*Factory)(nil)

// NewShardProcessor creates a new fixed shard processor
func (f *Factory) NewShardProcessor(shardID string) (*ShardProcessor, error) {
	return NewShardProcessor(
		shardID,
		f.timeSource,
		f.logger,
		f.shardLoadFn,
	), nil
}
