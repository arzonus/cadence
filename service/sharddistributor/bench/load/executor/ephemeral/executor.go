package ephemeral

import (
	"time"

	"github.com/uber-go/tally"
	"github.com/uber/cadence/common/clock"
	"github.com/uber/cadence/common/log"
	"github.com/uber/cadence/service/sharddistributor/bench/config"
	"github.com/uber/cadence/service/sharddistributor/bench/load/shardload"
	"github.com/uber/cadence/service/sharddistributor/client/clientcommon"
	"github.com/uber/cadence/service/sharddistributor/client/executorclient"
)

const (
	defaultEphemeralShardLifetime = 10 * time.Minute
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

	if cfg.EphemeralShardLifetime == 0 {
		cfg.EphemeralShardLifetime = defaultEphemeralShardLifetime
	}

	// Create factory
	factory := NewFactory(
		shardLoadFn,
		cfg.EphemeralShardLifetime,
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
		Metadata:              executorclient.ExecutorMetadata{"bench-executor": id, "type": "ephemeral"},
	}

	// Create executor with namespace
	return executorclient.NewExecutorWithNamespace(params, cfg.Name)
}
