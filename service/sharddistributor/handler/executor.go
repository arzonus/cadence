package handler

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/uber/cadence/common/clock"
	"github.com/uber/cadence/common/log"
	"github.com/uber/cadence/common/log/tag"
	"github.com/uber/cadence/common/metrics"
	"github.com/uber/cadence/common/types"
	"github.com/uber/cadence/service/sharddistributor/config"
	"github.com/uber/cadence/service/sharddistributor/store"
)

const (
	_heartbeatRefreshRate = 2 * time.Second

	_maxMetadataKeys      = 32
	_maxMetadataKeyLength = 128
	_maxMetadataValueSize = 512 * 1024 // 512KB
)

type executor struct {
	logger                 log.Logger
	timeSource             clock.TimeSource
	storage                store.Store
	shardDistributionCfg   config.ShardDistribution
	migrationConfiguration *config.MigrationConfig
	metricsClient          metrics.Client
}

func NewExecutorHandler(
	logger log.Logger,
	storage store.Store,
	timeSource clock.TimeSource,
	shardDistributionCfg config.ShardDistribution,
	migrationConfig *config.MigrationConfig,
	metricsClient metrics.Client,
) Executor {
	return &executor{
		logger:                 logger,
		timeSource:             timeSource,
		storage:                storage,
		shardDistributionCfg:   shardDistributionCfg,
		migrationConfiguration: migrationConfig,
		metricsClient:          metricsClient,
	}
}

func (h *executor) Heartbeat(ctx context.Context, request *types.ExecutorHeartbeatRequest) (*types.ExecutorHeartbeatResponse, error) {
	previousHeartbeat, assignedShards, err := h.storage.GetHeartbeat(ctx, request.Namespace, request.ExecutorID)
	// We ignore Executor not found errors, since it just means that this executor heartbeat the first time.
	if err != nil && !errors.Is(err, store.ErrExecutorNotFound) {
		return nil, fmt.Errorf("get heartbeat: %w", err)
	}

	heartbeatTime := h.timeSource.Now().UTC()
	mode := h.migrationConfiguration.GetMigrationMode(request.Namespace)

	switch mode {
	case types.MigrationModeINVALID:
		h.logger.Warn("Migration mode is invalid", tag.ShardNamespace(request.Namespace), tag.ShardExecutor(request.ExecutorID))
		return nil, fmt.Errorf("migration mode is invalid")
	case types.MigrationModeLOCALPASSTHROUGH:
		h.logger.Warn("Migration mode is local passthrough, no calls to heartbeat allowed", tag.ShardNamespace(request.Namespace), tag.ShardExecutor(request.ExecutorID))
		return nil, fmt.Errorf("migration mode is local passthrough")
	// From SD perspective the behaviour is the same
	case types.MigrationModeLOCALPASSTHROUGHSHADOW, types.MigrationModeDISTRIBUTEDPASSTHROUGH:
		assignedShards, err = h.assignShardsInCurrentHeartbeat(ctx, request)
		if err != nil {
			return nil, err
		}
	}

	// If the state has changed we need to update heartbeat data.
	// Otherwise, we want to do it with controlled frequency - at most every _heartbeatRefreshRate.
	if previousHeartbeat != nil && request.Status == previousHeartbeat.Status && mode == types.MigrationModeONBOARDED {
		lastHeartbeatTime := time.Unix(previousHeartbeat.LastHeartbeat, 0)
		if heartbeatTime.Sub(lastHeartbeatTime) < _heartbeatRefreshRate {
			return _convertResponse(assignedShards, mode), nil
		}
	}

	newHeartbeat := store.HeartbeatState{
		LastHeartbeat:  heartbeatTime.Unix(),
		Status:         request.Status,
		ReportedShards: request.ShardStatusReports,
		Metadata:       request.GetMetadata(),
	}

	if err := validateMetadata(newHeartbeat.Metadata); err != nil {
		return nil, fmt.Errorf("validate metadata: %w", err)
	}

	err = h.storage.RecordHeartbeat(ctx, request.Namespace, request.ExecutorID, newHeartbeat)
	if err != nil {
		return nil, fmt.Errorf("record heartbeat: %w", err)
	}

	// emits metrics in background to not block the heartbeat response
	go h.emitShardAssignmentMetrics(request.Namespace, heartbeatTime, previousHeartbeat, assignedShards)

	return _convertResponse(assignedShards, mode), nil
}

// emitShardAssignmentMetrics emits the following metrics for newly assigned shards:
// - ShardAssignmentDistributionLatency: time taken since the shard was assigned to heartbeat time
// - ShardHandoverLatency: time taken since the previous executor's last heartbeat to heartbeat time
func (h *executor) emitShardAssignmentMetrics(namespace string, heartbeatTime time.Time, previousHeartbeat *store.HeartbeatState, assignedState *store.AssignedState) {

	// find newly assigned shards, if there are none, no handovers happened
	newAssignedShardIDs := filterNewlyAssignedShardIDs(previousHeartbeat, assignedState)
	if len(newAssignedShardIDs) == 0 {
		// no handovers happened, nothing to do
		return
	}

	for _, shardID := range newAssignedShardIDs {
		h.emitShardAssignmentMetricsPerShard(namespace, shardID, heartbeatTime)
	}
}

func (h *executor) emitShardAssignmentMetricsPerShard(namespace string, shardID string, heartbeatTime time.Time) {
	stats, err := h.storage.GetShardStats(context.Background(), namespace, shardID)
	if err != nil {
		h.logger.Warn("Failed to get shard stats for handover latency metric",
			tag.ShardNamespace(namespace), tag.Error(err), tag.ShardKey(shardID))
		return
	}

	metricsScope := h.metricsClient.
		Scope(metrics.ShardDistributorHeartbeatScope).
		Tagged(metrics.NamespaceTag(namespace))

	distributionLatency := heartbeatTime.Sub(time.UnixMilli(stats.LastAssignmentTimeMs))
	metricsScope.RecordHistogramDuration(metrics.ShardDistributorShardAssignmentDistributionLatency, distributionLatency)

	if stats.PreviousExecutorLastHeartbeatTimeMs == nil || stats.LastHandoverType == nil {
		// this means that the shard was never assigned before, so no handover happened
		return
	}

	handoverLatency := heartbeatTime.Sub(time.UnixMilli(*stats.PreviousExecutorLastHeartbeatTimeMs))
	metricsScope.Tagged(metrics.HandoverTypeTag(stats.LastHandoverType.String())).
		RecordHistogramDuration(metrics.ShardDistributorShardHandoverLatency, handoverLatency)
}

// assignShardsInCurrentHeartbeat is used during the migration phase to assign the shards to the executors according to what is reported during the heartbeat
func (h *executor) assignShardsInCurrentHeartbeat(ctx context.Context, request *types.ExecutorHeartbeatRequest) (*store.AssignedState, error) {
	assignedShards := store.AssignedState{
		AssignedShards: make(map[string]*types.ShardAssignment),
		LastUpdated:    h.timeSource.Now().Unix(),
		ModRevision:    int64(0),
	}
	err := h.storage.DeleteExecutors(ctx, request.GetNamespace(), []string{request.GetExecutorID()}, store.NopGuard())
	if err != nil {
		return nil, fmt.Errorf("delete executors: %w", err)
	}
	for shard := range request.GetShardStatusReports() {
		assignedShards.AssignedShards[shard] = &types.ShardAssignment{
			Status: types.AssignmentStatusREADY,
		}
	}
	assignShardsRequest := store.AssignShardsRequest{
		ShardAssignments: map[string]store.AssignedState{
			request.GetExecutorID(): assignedShards,
		},
	}
	err = h.storage.AssignShards(ctx, request.GetNamespace(), assignShardsRequest, store.NopGuard())
	if err != nil {
		return nil, fmt.Errorf("assign shards in current heartbeat: %w", err)
	}
	return &assignedShards, nil
}

func _convertResponse(shards *store.AssignedState, mode types.MigrationMode) *types.ExecutorHeartbeatResponse {
	res := &types.ExecutorHeartbeatResponse{}
	res.MigrationMode = mode
	if shards == nil {
		return res
	}
	res.ShardAssignments = shards.AssignedShards
	return res
}

func validateMetadata(metadata map[string]string) error {
	if len(metadata) > _maxMetadataKeys {
		return fmt.Errorf("metadata has %d keys, which exceeds the maximum of %d", len(metadata), _maxMetadataKeys)
	}

	for key, value := range metadata {
		if len(key) > _maxMetadataKeyLength {
			return fmt.Errorf("metadata key %q has length %d, which exceeds the maximum of %d", key, len(key), _maxMetadataKeyLength)
		}

		if len(value) > _maxMetadataValueSize {
			return fmt.Errorf("metadata value for key %q has size %d bytes, which exceeds the maximum of %d bytes", key, len(value), _maxMetadataValueSize)
		}
	}

	return nil
}

func filterNewlyAssignedShardIDs(previousHeartbeat *store.HeartbeatState, assignedState *store.AssignedState) []string {
	// if assignedState is nil, no shards are assigned
	if assignedState == nil || len(assignedState.AssignedShards) == 0 {
		return nil
	}

	// if previousHeartbeat is nil, all assigned shards are new
	if previousHeartbeat == nil {
		var newAssignedShardIDs = make([]string, len(assignedState.AssignedShards))

		var i int
		for assignedShardID := range assignedState.AssignedShards {
			newAssignedShardIDs[i] = assignedShardID
			i++
		}

		return newAssignedShardIDs
	}

	// find shards that are assigned now but were not reported in the previous heartbeat
	var newAssignedShardIDs []string
	for assignedShardID := range assignedState.AssignedShards {
		if _, ok := previousHeartbeat.ReportedShards[assignedShardID]; !ok {
			newAssignedShardIDs = append(newAssignedShardIDs, assignedShardID)
		}
	}

	return newAssignedShardIDs
}
