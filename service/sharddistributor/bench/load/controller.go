package load

import (
	"context"
	"fmt"

	"github.com/uber-go/tally"
	"github.com/uber/cadence/common/clock"
	"github.com/uber/cadence/common/log"
	"github.com/uber/cadence/service/sharddistributor/bench/config"
	"github.com/uber/cadence/service/sharddistributor/bench/load/executor"
	"github.com/uber/cadence/service/sharddistributor/client/executorclient"
)

type Controller struct {
	logger              log.Logger
	executorControllers []*executor.Controller
}

type ControllerParams struct {
	Config         config.Config
	ExecutorClient executorclient.Client
	Logger         log.Logger
	MetricClient   tally.Scope
	TimeSource     clock.TimeSource
}

func NewController(p ControllerParams) (*Controller, error) {
	var executorControllers = make([]*executor.Controller, len(p.Config.Bench.Namespaces))

	for i, namespaceCfg := range p.Config.Bench.Namespaces {
		var err error
		executorControllers[i], err = executor.NewController(
			namespaceCfg,
			p.ExecutorClient,
			p.Logger,
			p.MetricClient,
			p.TimeSource,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create executor controller for namespace %s: %w", namespaceCfg.Name, err)
		}
	}

	return &Controller{
		executorControllers: executorControllers,
		logger:              p.Logger,
	}, nil
}

func (c *Controller) Start(ctx context.Context) {
	c.logger.Info("Starting controller")
	for _, execController := range c.executorControllers {
		execController.Start(ctx)
	}
}

func (c *Controller) Stop() {
	c.logger.Info("Stopping controller")
	for _, execController := range c.executorControllers {
		execController.Stop()
	}
}
