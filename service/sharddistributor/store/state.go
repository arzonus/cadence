package store

import (
	"time"

	"github.com/uber/cadence/common/types"
)

type HeartbeatState struct {
	// LastHeartbeat is the time of the last heartbeat received from the executor
	LastHeartbeat  time.Time
	Status         types.ExecutorStatus
	ReportedShards map[string]*types.ShardStatusReport
	Metadata       map[string]string
}

type AssignedState struct {
	// AssignedShards is the map of shard ID to shard assignment
	AssignedShards map[string]*types.ShardAssignment

	// UpdatedTime is the time we last updated this assignment
	UpdatedTime time.Time
	ModRevision int64
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
	SmoothedLoad float64

	// LastAssignmentTimeMs is the timestamp when the shard was last assigned
	LastAssignmentTime time.Time

	// PreviousExecutorLastHeartbeatTimeMs is the last heartbeat timestamp
	// of the previous executor before the handover.
	// If the shard has never been handed over, this field is nil.
	PreviousExecutorLastHeartbeatTime *time.Time

	// LastHandoverType indicates the type of handover that occurred during the last reassignment.
	// If the shard has never been handed over, this field is nil.
	LastHandoverType *types.HandoverType

	// UpdatedTime is the timestamp when ShardStatistics was updated
	UpdatedTime time.Time
}

type ShardOwner struct {
	ExecutorID string
	Metadata   map[string]string
}
