package store

import (
	"context"
	"fmt"
)

//go:generate mockgen -package $GOPACKAGE -source $GOFILE -destination=store_mock.go Store
//go:generate gowrap gen -g -p . -i Store -t ./wrappers/templates/metered.tmpl -o ./wrappers/metered/store_generated.go -v handler=Wrapped

var (
	// ErrExecutorNotFound is an error that is returned when queries executor is not registered in the storage.
	ErrExecutorNotFound = fmt.Errorf("executor not found")

	// ErrShardNotFound is an error that is returned when a shard does not exist.
	ErrShardNotFound = fmt.Errorf("shard not found")

	// ErrShardStatsNotFound is an error that is returned when shard statistics do not exist.
	ErrShardStatsNotFound = fmt.Errorf("shard stats not found")

	// ErrVersionConflict is an error that is returned if during operations some precondition failed.
	ErrVersionConflict = fmt.Errorf("version conflict")

	// ErrExecutorNotRunning is an error that is returned when shard is attempted to be assigned to a not running executor.
	ErrExecutorNotRunning = fmt.Errorf("executor not running")
)

type ErrShardAlreadyAssigned struct {
	ShardID    string
	AssignedTo string
}

func (e *ErrShardAlreadyAssigned) Error() string {
	return fmt.Sprintf("shard %s is already assigned to %s", e.ShardID, e.AssignedTo)
}

// Txn represents a generic, backend-agnostic transaction.
// It is used as a vehicle for the GuardFunc to operate on.
type Txn interface{}

// GuardFunc is a function that applies a transactional precondition.
// It takes a generic transaction, applies a backend-specific guard,
// and returns the modified transaction.
type GuardFunc func(Txn) (Txn, error)

// NopGuard is a no-op guard that can be used when no transactional
// check is required. It simply returns the transaction as-is.
func NopGuard() GuardFunc {
	return func(txn Txn) (Txn, error) {
		return txn, nil
	}
}

// AssignShardsRequest is a request to assign shards to executors, and remove unused shards.
type AssignShardsRequest struct {
	// ShardAssignments contains new assignments of shards to executors.
	// Key: ExecutorID
	ShardAssignments map[string]AssignedState

	// ShardsStats contains statistics for shards to be updated.
	// This field is optional and can be nil if no statistics need to be updated.
	// Key: ShardID
	ShardStats map[string]ShardStatistics
}

// Store is a composite interface that combines all storage capabilities.
type Store interface {
	GetState(ctx context.Context, namespace string) (*NamespaceState, error)

	AssignShards(ctx context.Context, namespace string, request AssignShardsRequest, guard GuardFunc) error
	AssignShard(ctx context.Context, namespace, shardID, executorID string) error

	Subscribe(ctx context.Context, namespace string) (<-chan int64, error)
	DeleteExecutors(ctx context.Context, namespace string, executorIDs []string, guard GuardFunc) error

	// GetShardOwner retrieves the owner of a specific shard within a namespace.
	// It returns ErrShardNotFound if the shard does not exist.
	GetShardOwner(ctx context.Context, namespace, shardID string) (*ShardOwner, error)
	SubscribeToAssignmentChanges(ctx context.Context, namespace string) (<-chan map[*ShardOwner][]string, func(), error)

	GetHeartbeat(ctx context.Context, namespace string, executorID string) (*HeartbeatState, *AssignedState, error)
	RecordHeartbeat(ctx context.Context, namespace, executorID string, state HeartbeatState) error

	// GetShardStats retrieves statistics for a specific shard within a namespace.
	// It returns ErrShardStatsNotFound if no statistics are found for the given shard.
	GetShardStats(ctx context.Context, namespace string, shardID string) (*ShardStatistics, error)
	DeleteShardStats(ctx context.Context, namespace string, shardIDs []string, guard GuardFunc) error
}
