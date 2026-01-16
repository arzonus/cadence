package fixed

import (
	"context"
	"sync"
	"time"

	"github.com/uber/cadence/common/clock"
	"github.com/uber/cadence/common/log"
	"github.com/uber/cadence/common/log/tag"
	"github.com/uber/cadence/common/types"
	"github.com/uber/cadence/service/sharddistributor/client/executorclient"
)

const (
	processInterval = 10 * time.Second
)

// ShardProcessor processes a fixed shard for load testing
// Fixed shards never expire and always report READY status
type ShardProcessor struct {
	shardID      string
	timeSource   clock.TimeSource
	logger       log.Logger
	shardLoadFn  func(shardID string) float64
	stopChan     chan struct{}
	goRoutineWg  sync.WaitGroup
	processSteps int
}

// NewShardProcessor creates a new fixed shard processor
func NewShardProcessor(
	shardID string,
	timeSource clock.TimeSource,
	logger log.Logger,
	shardLoadFn func(shardID string) float64,
) *ShardProcessor {
	return &ShardProcessor{
		shardID:     shardID,
		timeSource:  timeSource,
		logger:      logger.WithTags(tag.ShardKey(shardID)),
		shardLoadFn: shardLoadFn,
		stopChan:    make(chan struct{}),
	}
}

var _ executorclient.ShardProcessor = (*ShardProcessor)(nil)

// Start starts the shard processor
func (p *ShardProcessor) Start(_ context.Context) error {
	p.logger.Info("Starting shard processor")
	p.goRoutineWg.Add(1)
	go p.process()
	return nil
}

func (p *ShardProcessor) process() {
	defer p.goRoutineWg.Done()

	ticker := p.timeSource.NewTicker(processInterval)
	defer ticker.Stop()

	for {
		select {
		case <-p.stopChan:
			return

		case <-ticker.Chan():
			p.processSteps++
			p.logger.Info("Processing shard",
				tag.Dynamic("steps", p.processSteps),
				tag.Dynamic("shardLoad", p.shardLoadFn(p.shardID)),
			)
		}
	}
}

// Stop stops the shard processor
func (p *ShardProcessor) Stop() {
	p.logger.Debug("Stopping fixed shard processor", tag.Dynamic("steps", p.processSteps))

	select {
	case <-p.stopChan:
		// Already stopped
		return
	default:
		close(p.stopChan)
	}

	p.goRoutineWg.Wait()
}

// GetShardReport returns the current shard report
// Fixed shards always report READY status
func (p *ShardProcessor) GetShardReport() executorclient.ShardReport {
	return executorclient.ShardReport{
		ShardLoad: p.shardLoadFn(p.shardID),
		Status:    types.ShardStatusREADY,
	}
}
