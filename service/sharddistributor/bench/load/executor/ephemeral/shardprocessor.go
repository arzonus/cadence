package ephemeral

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

// This is a shard processor, the only thing it currently does it
// count the number of steps it has processed and log that information
// over a fixed lifetime. After the lifetime ends, the processor stops itself.
const (
	processInterval = 10 * time.Second
)

var _ executorclient.ShardProcessor = (*ShardProcessor)(nil)

// NewShardProcessor creates a new ShardProcessor.
func NewShardProcessor(
	shardID string,
	timeSource clock.TimeSource,
	logger log.Logger,
	shardLoadFn func(string) float64,
	lifetime time.Duration,
) *ShardProcessor {
	return &ShardProcessor{
		shardID:     shardID,
		shardLoadFn: shardLoadFn,
		timeSource:  timeSource,
		logger:      logger.WithTags(tag.ShardKey(shardID)),
		stopChan:    make(chan struct{}),
		lifetime:    lifetime,
		status:      types.ShardStatusREADY,
	}
}

type ShardProcessor struct {
	mx           sync.RWMutex
	shardID      string
	logger       log.Logger
	shardLoadFn  func(string) float64
	stopChan     chan struct{}
	goRoutineWg  sync.WaitGroup
	timeSource   clock.TimeSource
	lifetime     time.Duration
	status       types.ShardStatus
	processSteps int
}

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

	deadTimer := p.timeSource.NewTicker(p.lifetime)
	defer deadTimer.Stop()

	for {
		select {
		case <-p.stopChan:
			return

		case <-deadTimer.Chan():
			p.logger.Info("Shard processor lifetime ended, changing status to Done",
				tag.Dynamic("steps", p.processSteps))

			p.mx.Lock()
			p.status = types.ShardStatusDONE
			p.mx.Unlock()

		case <-ticker.Chan():
			p.processSteps++
			p.logger.Info("Processing shard",
				tag.Dynamic("steps", p.processSteps),
				tag.Dynamic("shardLoad", p.shardLoadFn(p.shardID)),
			)
		}
	}
}

func (p *ShardProcessor) Stop() {
	p.logger.Info("Stopping shard processor", tag.Dynamic("steps", p.processSteps))
	close(p.stopChan)
	p.goRoutineWg.Wait()
}

func (p *ShardProcessor) GetShardReport() executorclient.ShardReport {
	p.mx.RLock()
	defer p.mx.RUnlock()

	return executorclient.ShardReport{
		ShardLoad: p.shardLoadFn(p.shardID),
		Status:    p.status,
	}
}
