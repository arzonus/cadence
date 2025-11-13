package etcdtypes

import (
	"github.com/uber/cadence/common/types"
	"github.com/uber/cadence/service/sharddistributor/store"
)

type AssignedState struct {
	AssignedShards map[string]*types.ShardAssignment `json:"assigned_shards"`
	UpdatedTime    Time                              `json:"updated_time"`
	ModRevision    int64                             `json:"mod_revision"`
}

// ToAssignedState converts the current AssignedState to store.AssignedState.
func (s *AssignedState) ToAssignedState() *store.AssignedState {
	if s == nil {
		return nil
	}

	return &store.AssignedState{
		AssignedShards: s.AssignedShards,
		UpdatedTime:    s.UpdatedTime.ToTime(),
		ModRevision:    s.ModRevision,
	}
}

// FromAssignedState creates an AssignedState from a store.AssignedState.
func FromAssignedState(src *store.AssignedState) *AssignedState {
	if src == nil {
		return nil
	}

	return &AssignedState{
		AssignedShards: src.AssignedShards,
		UpdatedTime:    Time(src.UpdatedTime),
		ModRevision:    src.ModRevision,
	}
}

type ShardStatistics struct {
	SmoothedLoad                      float64             `json:"smoothed_load"`
	LastAssignmentTime                Time                `json:"last_assignment_time"`
	PreviousExecutorLastHeartbeatTime *Time               `json:"previous_executor_last_heartbeat_time,omitempty"`
	LastHandoverType                  *types.HandoverType `json:"last_handover_type,omitempty"`
	UpdatedTime                       Time                `json:"updated_time"`
}

// ToShardStatistics converts the current ShardStatistics to store.ShardStatistics.
func (s *ShardStatistics) ToShardStatistics() *store.ShardStatistics {
	if s == nil {
		return nil
	}
	return &store.ShardStatistics{
		SmoothedLoad:                      s.SmoothedLoad,
		LastAssignmentTime:                s.LastAssignmentTime.ToTime(),
		PreviousExecutorLastHeartbeatTime: s.PreviousExecutorLastHeartbeatTime.ToTimePtr(),
		LastHandoverType:                  s.LastHandoverType,
		UpdatedTime:                       s.UpdatedTime.ToTime(),
	}
}

// FromShardStatistics creates a ShardStatistics from a store.ShardStatistics.
func FromShardStatistics(src *store.ShardStatistics) *ShardStatistics {
	if src == nil {
		return nil
	}

	return &ShardStatistics{
		SmoothedLoad:                      src.SmoothedLoad,
		LastAssignmentTime:                Time(src.LastAssignmentTime),
		PreviousExecutorLastHeartbeatTime: ToTimePtr(src.PreviousExecutorLastHeartbeatTime),
		LastHandoverType:                  src.LastHandoverType,
		UpdatedTime:                       Time(src.UpdatedTime),
	}
}
