package fixed

import (
	"github.com/uber-go/tally"
	"github.com/uber/cadence/common/clock"
	"github.com/uber/cadence/common/log"
	"github.com/uber/cadence/service/sharddistributor/bench/config"
	"github.com/uber/cadence/service/sharddistributor/bench/load/shardload"
	"github.com/uber/cadence/service/sharddistributor/client/clientcommon"
	"github.com/uber/cadence/service/sharddistributor/client/executorclient"
)

// NewExecutor creates a new fixed shard executor
func NewExecutor(
	id string,
	cfg config.NamespaceConfig,
	client executorclient.Client,
	logger log.Logger,
	metricsScope tally.Scope,
	timeSource clock.TimeSource,
) (executorclient.Executor[*ShardProcessor], error) {

	shardLoadFn, err := shardload.Fn(cfg.ShardLoadAlgorithm)
	if err != nil {
		shardLoadFn = shardload.Constant
	}

	// Create factory
	factory := NewFactory(
		shardLoadFn,
		timeSource,
		logger,
	)

	// Create executor client config
	clientConfig := clientcommon.Config{
		Namespaces: []clientcommon.NamespaceConfig{
			{Namespace: cfg.Name},
		},
	}

	// Create executor parameters
	params := executorclient.Params[*ShardProcessor]{
		ExecutorClient:        client,
		MetricsScope:          metricsScope,
		Logger:                logger,
		ShardProcessorFactory: factory,
		Config:                clientConfig,
		TimeSource:            timeSource,
		Metadata:              executorclient.ExecutorMetadata{"bench-executor": id, "type": "fixed"},
	}

	// Create executor with namespace
	return executorclient.NewExecutorWithNamespace(params, cfg.Name)
}
