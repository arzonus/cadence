package warmstorage

import (
	"fmt"

	"github.com/uber/cadence/common/constants"
	"github.com/uber/cadence/common/persistence"
	"github.com/uber/cadence/common/persistence/serialization"
)

// toBlob packs a closed run into the slim row. It reuses the blob SQL already stores for executions,
// minus auto-reset points (Describe rebuilds them) and minus everything that only matters while running.
func toBlob(parser serialization.Parser, state *persistence.InternalWorkflowMutableState) (*persistence.DataBlob, error) {
	if state.VersionHistories == nil {
		// NextEventID is derived from version histories on read, so legacy runs without them stay hot
		return nil, fmt.Errorf("run has no version histories")
	}
	info := serialization.FromInternalWorkflowExecutionInfo(state.ExecutionInfo)
	info.AutoResetPoints = nil
	info.AutoResetPointsEncoding = ""
	info.VersionHistories = state.VersionHistories.Data
	info.VersionHistoriesEncoding = string(state.VersionHistories.Encoding)
	blob, err := parser.WorkflowExecutionInfoToBlob(info)
	if err != nil {
		return nil, err
	}
	return &blob, nil
}

// fromBlob rebuilds the state the stores would return for the run, with empty pending-work maps.
func fromBlob(
	parser serialization.Parser,
	serializer persistence.PayloadSerializer,
	key persistence.ClosedExecutionKey,
	blob *persistence.DataBlob,
) (*persistence.InternalWorkflowMutableState, error) {
	info, err := parser.WorkflowExecutionInfoFromBlob(blob.Data, string(blob.Encoding))
	if err != nil {
		return nil, err
	}
	executionInfo := serialization.ToInternalWorkflowExecutionInfo(info)
	executionInfo.DomainID = key.DomainID
	executionInfo.WorkflowID = key.WorkflowID
	executionInfo.RunID = key.RunID
	executionInfo.AutoResetPointsDropped = true

	versionHistoriesBlob := persistence.NewDataBlob(info.GetVersionHistories(), constants.EncodingType(info.GetVersionHistoriesEncoding()))
	versionHistories, err := serializer.DeserializeVersionHistories(versionHistoriesBlob)
	if err != nil {
		return nil, err
	}
	if versionHistories == nil {
		return nil, fmt.Errorf("slim row for run %v has no version histories", key.RunID)
	}
	current, err := persistence.NewVersionHistoriesFromInternalType(versionHistories).GetCurrentVersionHistory()
	if err != nil {
		return nil, err
	}
	lastItem, err := current.GetLastItem()
	if err != nil {
		return nil, err
	}
	// a closed run's last event is the last item of its current branch
	executionInfo.NextEventID = lastItem.EventID + 1

	return &persistence.InternalWorkflowMutableState{
		ExecutionInfo:       executionInfo,
		VersionHistories:    versionHistoriesBlob,
		ActivityInfos:       map[int64]*persistence.InternalActivityInfo{},
		TimerInfos:          map[string]*persistence.TimerInfo{},
		ChildExecutionInfos: map[int64]*persistence.InternalChildExecutionInfo{},
		RequestCancelInfos:  map[int64]*persistence.RequestCancelInfo{},
		SignalInfos:         map[int64]*persistence.SignalInfo{},
		SemaphoreInfos:      map[int64]*persistence.SemaphoreInfo{},
		SignalRequestedIDs:  map[string]struct{}{},
	}, nil
}
