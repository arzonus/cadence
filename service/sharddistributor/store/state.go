package store

import (
	"github.com/uber/cadence/common/types"
)

type HeartbeatState struct {
	LastHeartbeat  int64                               `json:"last_heartbeat"`
	Status         types.ExecutorStatus                `json:"status"`
	ReportedShards map[string]*types.ShardStatusReport `json:"reported_shards"`
	Metadata       map[string]string                   `json:"metadata"`
}

type AssignedState struct {
	AssignedShards map[string]*types.ShardAssignment `json:"assigned_shards"` // What we assigned
	LastUpdated    int64                             `json:"last_updated"`
	ModRevision    int64                             `json:"mod_revision"`
}

type NamespaceState struct {
	// Executors holds the heartbeat states of all executors in the namespace.
	// Key: ExecutorID
	Executors map[string]HeartbeatState

	// ShardStats holds the statistics of all shards in the namespace.
	// Key: ShardID
	ShardStats map[string]ShardStatistics

	// ShardAssignments holds the assignment states of all shards in the namespace.
	// Key: ExecutorID
	ShardAssignments map[string]AssignedState
	GlobalRevision   int64
}

type ShardState struct {
	ExecutorID string
}

// ShardStatistics holds statistics information about a shard.
// This information is stored and updated by a leader together with shard assignments.
type ShardStatistics struct {
	// SmoothedLoad is EWMA of shard load that persists across executor changes
	SmoothedLoad float64 `json:"smoothed_load"`

	// LastAssignmentTimeMs is the timestamp (unix milliseconds) when the shard was last assigned
	LastAssignmentTimeMs int64 `json:"last_assignment_time_ms"`

	// PreviousExecutorLastHeartbeatTimeMs is the last heartbeat timestamp (unix milliseconds)
	// of the previous executor before the handover.
	// If the shard has never been handed over, this field is nil.
	PreviousExecutorLastHeartbeatTimeMs *int64 `json:"previous_executor_last_heartbeat_time_ms"`

	// LastHandoverType indicates the type of handover that occurred during the last reassignment.
	// If the shard has never been handed over, this field is nil.
	LastHandoverType *types.HandoverType `json:"last_handover_type"`

	// UpdateTime is the timestamp (unix milliseconds) when ShardStatistics was updated
	UpdateTimeMs int64 `json:"update_time_ms"`
}

type ShardOwner struct {
	ExecutorID string
	Metadata   map[string]string
}
