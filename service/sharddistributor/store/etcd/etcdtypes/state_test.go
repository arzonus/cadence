package etcdtypes

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/uber/cadence/common/types"
	"github.com/uber/cadence/service/sharddistributor/store"
)

func TestAssignedState_FieldNumberMatched(t *testing.T) {
	require.Equal(t,
		reflect.TypeOf(AssignedState{}).NumField(),
		reflect.TypeOf(store.AssignedState{}).NumField(),
		"AssignedState field count mismatch with store.AssignedState; ensure conversion is updated",
	)
}

func TestAssignedState_ToAssignedState(t *testing.T) {
	tests := map[string]struct {
		input  *AssignedState
		expect *store.AssignedState
	}{
		"nil": {
			input:  nil,
			expect: nil,
		},
		"success": {
			input: &AssignedState{
				AssignedShards: map[string]*types.ShardAssignment{
					"1": {Status: types.AssignmentStatusREADY},
				},
				UpdatedTime: Time(time.Date(2025, 11, 18, 12, 0, 0, 123456789, time.UTC)),
				ModRevision: 42,
			},
			expect: &store.AssignedState{
				AssignedShards: map[string]*types.ShardAssignment{
					"1": {Status: types.AssignmentStatusREADY},
				},
				UpdatedTime: time.Date(2025, 11, 18, 12, 0, 0, 123456789, time.UTC),
				ModRevision: 42,
			},
		},
	}

	for name, c := range tests {
		t.Run(name, func(t *testing.T) {
			got := c.input.ToAssignedState()

			if c.expect == nil {
				require.Nil(t, got)
				return
			}

			require.NotNil(t, got)
			require.Equal(t, len(c.input.AssignedShards), len(got.AssignedShards))
			for k := range c.input.AssignedShards {
				require.Equal(t, c.input.AssignedShards[k].Status, got.AssignedShards[k].Status)
			}
			require.Equal(t, time.Time(c.input.UpdatedTime).UnixNano(), got.UpdatedTime.UnixNano())
			require.Equal(t, c.input.ModRevision, got.ModRevision)
		})
	}
}
func TestAssignedState_FromAssignedState(t *testing.T) {
	tests := map[string]struct {
		input  *store.AssignedState
		expect *AssignedState
	}{
		"nil": {
			input:  nil,
			expect: nil,
		},
		"success": {
			input: &store.AssignedState{
				AssignedShards: map[string]*types.ShardAssignment{
					"9": {Status: types.AssignmentStatusREADY},
				},
				UpdatedTime: time.Date(2025, 11, 18, 13, 0, 0, 987654321, time.UTC),
				ModRevision: 77,
			},
			expect: &AssignedState{
				AssignedShards: map[string]*types.ShardAssignment{
					"9": {Status: types.AssignmentStatusREADY},
				},
				UpdatedTime: Time(time.Date(2025, 11, 18, 13, 0, 0, 987654321, time.UTC)),
				ModRevision: 77,
			},
		},
	}

	for name, c := range tests {
		t.Run(name, func(t *testing.T) {
			got := FromAssignedState(c.input)
			if c.expect == nil {
				require.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			require.Equal(t, len(c.input.AssignedShards), len(got.AssignedShards))
			for k := range c.input.AssignedShards {
				require.Equal(t, c.input.AssignedShards[k].Status, got.AssignedShards[k].Status)
			}
			require.Equal(t, c.input.UpdatedTime.UnixNano(), time.Time(got.UpdatedTime).UnixNano())
			require.Equal(t, c.input.ModRevision, got.ModRevision)
		})
	}
}

func TestAssignedState_JSONMarshalling(t *testing.T) {
	const jsonStr = `{"assigned_shards":{"1":{"status":"READY"}},"updated_time":"2025-11-18T12:00:00.123456789Z","mod_revision":42}`

	state := &AssignedState{
		AssignedShards: map[string]*types.ShardAssignment{
			"1": {Status: types.AssignmentStatusREADY},
		},
		UpdatedTime: Time(time.Date(2025, 11, 18, 12, 0, 0, 123456789, time.UTC)),
		ModRevision: 42,
	}

	// Marshal to JSON
	b, err := json.Marshal(state)
	require.NoError(t, err)
	require.JSONEq(t, jsonStr, string(b))

	// Unmarshal from JSON
	var unmarshalled AssignedState
	err = json.Unmarshal([]byte(jsonStr), &unmarshalled)
	require.NoError(t, err)
	require.Equal(t, state.AssignedShards["1"].Status, unmarshalled.AssignedShards["1"].Status)
	require.Equal(t, time.Time(state.UpdatedTime).UnixNano(), time.Time(unmarshalled.UpdatedTime).UnixNano())
	require.Equal(t, state.ModRevision, unmarshalled.ModRevision)
}
func TestShardStatistics_FieldNumberMatched(t *testing.T) {
	require.Equal(t,
		reflect.TypeOf(ShardStatistics{}).NumField(),
		reflect.TypeOf(store.ShardStatistics{}).NumField(),
		"ShardStatistics field count mismatch with store.ShardStatistics; ensure conversion is updated",
	)
}

func TestShardStatistics_ToShardStatistics(t *testing.T) {
	// extended to include optional fields

	prevHeartbeat := Time(time.Date(2025, 11, 18, 13, 30, 0, 999999999, time.UTC))
	tests := map[string]struct {
		input  *ShardStatistics
		expect *store.ShardStatistics
	}{
		"nil": {input: nil, expect: nil},
		"only required fields": {
			input: &ShardStatistics{
				SmoothedLoad:       12.34,
				LastAssignmentTime: Time(time.Date(2025, 11, 18, 14, 0, 0, 111111111, time.UTC)),
				UpdatedTime:        Time(time.Date(2025, 11, 18, 15, 0, 0, 222222222, time.UTC)),
			},
			expect: &store.ShardStatistics{
				SmoothedLoad:       12.34,
				LastAssignmentTime: time.Date(2025, 11, 18, 14, 0, 0, 111111111, time.UTC),
				UpdatedTime:        time.Date(2025, 11, 18, 15, 0, 0, 222222222, time.UTC),
			},
		},
		"all fields": {
			input: &ShardStatistics{
				SmoothedLoad:                      99.99,
				LastAssignmentTime:                Time(time.Date(2025, 11, 18, 14, 10, 0, 333333333, time.UTC)),
				PreviousExecutorLastHeartbeatTime: &prevHeartbeat,
				LastHandoverType:                  types.HandoverTypeGRACEFUL.Ptr(),
				UpdatedTime:                       Time(time.Date(2025, 11, 18, 16, 0, 0, 444444444, time.UTC)),
			},
			expect: &store.ShardStatistics{
				SmoothedLoad:                      99.99,
				LastAssignmentTime:                time.Date(2025, 11, 18, 14, 10, 0, 333333333, time.UTC),
				PreviousExecutorLastHeartbeatTime: prevHeartbeat.ToTimePtr(),
				LastHandoverType:                  types.HandoverTypeGRACEFUL.Ptr(),
				UpdatedTime:                       time.Date(2025, 11, 18, 16, 0, 0, 444444444, time.UTC),
			},
		},
	}

	for name, c := range tests {
		t.Run(name, func(t *testing.T) {
			got := c.input.ToShardStatistics()
			if c.expect == nil {
				require.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			require.Equal(t, c.input.SmoothedLoad, got.SmoothedLoad)
			require.Equal(t, time.Time(c.input.LastAssignmentTime).UnixNano(), got.LastAssignmentTime.UnixNano())
			if c.input.PreviousExecutorLastHeartbeatTime != nil {
				require.NotNil(t, got.PreviousExecutorLastHeartbeatTime)
				require.Equal(t, time.Time(*c.input.PreviousExecutorLastHeartbeatTime).UnixNano(), got.PreviousExecutorLastHeartbeatTime.UnixNano())
			} else {
				require.Nil(t, got.PreviousExecutorLastHeartbeatTime)
			}
			if c.input.LastHandoverType != nil {
				require.NotNil(t, got.LastHandoverType)
				require.Equal(t, *c.input.LastHandoverType, *got.LastHandoverType)
			} else {
				require.Nil(t, got.LastHandoverType)
			}
			require.Equal(t, time.Time(c.input.UpdatedTime).UnixNano(), got.UpdatedTime.UnixNano())
		})
	}
}

func TestShardStatistics_FromShardStatistics(t *testing.T) {
	// extended to include optional fields
	handoverType := types.HandoverTypeEMERGENCY.Ptr()
	prevHeartbeat := time.Date(2025, 11, 18, 18, 30, 0, 555555555, time.UTC)
	tests := map[string]struct {
		input  *store.ShardStatistics
		expect *ShardStatistics
	}{
		"nil": {input: nil, expect: nil},
		"only required fields": {
			input: &store.ShardStatistics{
				SmoothedLoad:       1.23,
				LastAssignmentTime: time.Date(2025, 11, 18, 19, 0, 0, 666666666, time.UTC),
				UpdatedTime:        time.Date(2025, 11, 18, 20, 0, 0, 777777777, time.UTC),
			},
			expect: &ShardStatistics{
				SmoothedLoad:       1.23,
				LastAssignmentTime: Time(time.Date(2025, 11, 18, 19, 0, 0, 666666666, time.UTC)),
				UpdatedTime:        Time(time.Date(2025, 11, 18, 20, 0, 0, 777777777, time.UTC)),
			},
		},
		"all fields": {
			input: &store.ShardStatistics{
				SmoothedLoad:                      55.55,
				LastAssignmentTime:                time.Date(2025, 11, 18, 21, 0, 0, 888888888, time.UTC),
				PreviousExecutorLastHeartbeatTime: &prevHeartbeat,
				LastHandoverType:                  handoverType,
				UpdatedTime:                       time.Date(2025, 11, 18, 22, 0, 0, 999999999, time.UTC),
			},
			expect: &ShardStatistics{
				SmoothedLoad:                      55.55,
				LastAssignmentTime:                Time(time.Date(2025, 11, 18, 21, 0, 0, 888888888, time.UTC)),
				PreviousExecutorLastHeartbeatTime: ToTimePtr(&prevHeartbeat),
				LastHandoverType:                  handoverType,
				UpdatedTime:                       Time(time.Date(2025, 11, 18, 22, 0, 0, 999999999, time.UTC)),
			},
		},
	}
	for name, c := range tests {
		t.Run(name, func(t *testing.T) {
			got := FromShardStatistics(c.input)
			if c.expect == nil {
				require.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			require.InDelta(t, c.input.SmoothedLoad, got.SmoothedLoad, 1e-9)
			require.Equal(t, c.input.LastAssignmentTime.UnixNano(), time.Time(got.LastAssignmentTime).UnixNano())
			if c.input.PreviousExecutorLastHeartbeatTime != nil {
				require.NotNil(t, got.PreviousExecutorLastHeartbeatTime)
				require.Equal(t, c.input.PreviousExecutorLastHeartbeatTime.UnixNano(), time.Time(*got.PreviousExecutorLastHeartbeatTime).UnixNano())
			} else {
				require.Nil(t, got.PreviousExecutorLastHeartbeatTime)
			}
			if c.input.LastHandoverType != nil {
				require.NotNil(t, got.LastHandoverType)
				require.Equal(t, *c.input.LastHandoverType, *got.LastHandoverType)
			} else {
				require.Nil(t, got.LastHandoverType)
			}
			require.Equal(t, c.input.UpdatedTime.UnixNano(), time.Time(got.UpdatedTime).UnixNano())
		})
	}
}

func TestShardStatistics_JSONMarshalling(t *testing.T) {
	const jsonStr = `{"smoothed_load":12.34,"last_assignment_time":"2025-11-18T14:00:00.111111111Z","previous_executor_last_heartbeat_time":"2025-11-18T16:30:00.111111111Z","last_handover_type":"GRACEFUL","updated_time":"2025-11-18T15:00:00.222222222Z"}`

	unmarshalled := &ShardStatistics{}
	err := json.Unmarshal([]byte(jsonStr), unmarshalled)
	require.NoError(t, err)

	prevHeartbeat := Time(time.Date(2025, 11, 18, 16, 30, 0, 111111111, time.UTC))
	expected := &ShardStatistics{
		SmoothedLoad:                      12.34,
		LastAssignmentTime:                Time(time.Date(2025, 11, 18, 14, 0, 0, 111111111, time.UTC)),
		PreviousExecutorLastHeartbeatTime: &prevHeartbeat,
		LastHandoverType:                  types.HandoverTypeGRACEFUL.Ptr(),
		UpdatedTime:                       Time(time.Date(2025, 11, 18, 15, 0, 0, 222222222, time.UTC)),
	}
	bAll, err := json.Marshal(expected)
	require.NoError(t, err)
	require.JSONEq(t, jsonStr, string(bAll))
	require.Equal(t, expected, unmarshalled)

}
