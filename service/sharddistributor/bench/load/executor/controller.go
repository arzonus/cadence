package executor

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/uber-go/tally"
	"github.com/uber/cadence/common/clock"
	"github.com/uber/cadence/common/log"
	"github.com/uber/cadence/common/log/tag"
	"github.com/uber/cadence/service/sharddistributor/bench/config"
	"github.com/uber/cadence/service/sharddistributor/bench/load/executor/ephemeral"
	"github.com/uber/cadence/service/sharddistributor/bench/load/executor/fixed"
	"github.com/uber/cadence/service/sharddistributor/client/executorclient"
)

type Controller struct {
	cfg        config.NamespaceConfig
	executors  []Executor
	timeSource clock.TimeSource
	logger     log.Logger

	stopChan    chan struct{}
	goRoutineWg sync.WaitGroup
}

func NewController(
	cfg config.NamespaceConfig,
	client executorclient.Client,
	logger log.Logger,
	metricsScope tally.Scope,
	timeSource clock.TimeSource,
) (*Controller, error) {
	var executors = make([]Executor, cfg.NumExecutors)

	for i := uint(0); i < cfg.NumExecutors; i++ {
		var err error

		id := fmt.Sprintf("%s-executor-%d", cfg.Name, i+1)

		switch cfg.Type {
		case "fixed":
			executors[i], err = fixed.NewExecutor(
				id, cfg, client,
				logger,
				metricsScope, timeSource,
			)
		case "ephemeral":
			executors[i], err = ephemeral.NewExecutor(
				id, cfg, client,
				logger,
				metricsScope, timeSource)
		default:
			return nil, fmt.Errorf("unknown executor type: %s", cfg.Type)
		}

		if err != nil {
			return nil, fmt.Errorf("failed to create executor: %w", err)
		}
	}

	if cfg.ExecutorRecreationInterval <= 0 {
		cfg.ExecutorRecreationInterval = time.Minute
	}

	return &Controller{
		cfg:        cfg,
		executors:  executors,
		timeSource: timeSource,
		logger:     logger.WithTags(tag.ShardNamespace(cfg.Name)),
	}, nil
}

func (c *Controller) Start(ctx context.Context) {
	c.logger.Info("Starting executor controller")

	for _, executor := range c.executors {
		executor.Start(ctx)
	}
	c.goRoutineWg.Add(1)
	go c.recreationLoop()
}

func (c *Controller) Stop() {
	c.logger.Info("Stopping executor controller")
	for _, executor := range c.executors {
		executor.Stop()
	}
	close(c.stopChan)
	c.goRoutineWg.Wait()
}

func (c *Controller) recreationLoop() {
	defer c.goRoutineWg.Done()

	if c.cfg.ExecutorRecreationPercentage <= 0 {
		c.logger.Info("Recreation of executors disabled")
		return
	}

	ticker := c.timeSource.NewTicker(c.cfg.ExecutorRecreationInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopChan:
			return
		case <-ticker.Chan():
			c.logger.Info("Recreating executors as per configuration", tag.Dynamic("percentage", c.cfg.ExecutorRecreationPercentage))

			// TODO: Implement the logic to recreate the specified percentage of executors
		}
	}
}

// Executor defines the interface for an executor instance
type Executor interface {
	Start(ctx context.Context)
	Stop()
}
