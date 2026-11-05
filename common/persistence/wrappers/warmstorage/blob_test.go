package warmstorage

import (
	"testing"
	"time"

	fuzz "github.com/google/gofuzz"
	"github.com/pborman/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/uber/cadence/common/constants"
	"github.com/uber/cadence/common/persistence"
	"github.com/uber/cadence/common/persistence/serialization"
	"github.com/uber/cadence/common/types"
	"github.com/uber/cadence/common/types/mapper/testutils"
)

const lastEventID = 42

func testParser(t *testing.T) serialization.Parser {
	parser, err := serialization.NewParser(persistence.NewDefaultDynamicConfiguration())
	require.NoError(t, err)
	return parser
}

func testVersionHistories(t *testing.T) *persistence.DataBlob {
	blob, err := persistence.NewPayloadSerializer().SerializeVersionHistories(&types.VersionHistories{
		Histories: []*types.VersionHistory{{
			BranchToken: []byte("branch"),
			Items:       []*types.VersionHistoryItem{{EventID: 10, Version: 1}, {EventID: lastEventID, Version: 2}},
		}},
	}, constants.EncodingTypeThriftRW)
	require.NoError(t, err)
	return blob
}

func testKey() persistence.ClosedExecutionKey {
	return persistence.ClosedExecutionKey{DomainID: uuid.New(), WorkflowID: "wf", RunID: uuid.New()}
}

// closedState is a closed run as a store returns it, including the parts warm storage drops.
func closedState(t *testing.T, key persistence.ClosedExecutionKey) *persistence.InternalWorkflowMutableState {
	return &persistence.InternalWorkflowMutableState{
		ExecutionInfo: &persistence.InternalWorkflowExecutionInfo{
			DomainID:               key.DomainID,
			WorkflowID:             key.WorkflowID,
			RunID:                  key.RunID,
			FirstExecutionRunID:    uuid.New(),
			ParentDomainID:         uuid.New(),
			ParentWorkflowID:       "parent",
			ParentRunID:            uuid.New(),
			InitiatedID:            5,
			CompletionEventBatchID: 40,
			CompletionEvent:        persistence.NewDataBlob([]byte("completion"), constants.EncodingTypeThriftRW),
			TaskList:               "tl",
			WorkflowTypeName:       "type",
			WorkflowTimeout:        time.Hour,
			State:                  persistence.WorkflowStateCompleted,
			CloseStatus:            persistence.WorkflowCloseStatusCompleted,
			NextEventID:            lastEventID + 1,
			LastFirstEventID:       41,
			LastProcessedEvent:     39,
			StartTimestamp:         time.UnixMilli(1752018142820),
			LastUpdatedTimestamp:   time.UnixMilli(1752018142821),
			CreateRequestID:        "request",
			DecisionScheduleID:     constants.EmptyEventID,
			DecisionStartedID:      constants.EmptyEventID,
			AutoResetPoints:        persistence.NewDataBlob([]byte("reset points"), constants.EncodingTypeThriftRW),
			HasRetryPolicy:         true,
			MaximumAttempts:        3,
			CronSchedule:           "* * * * *",
			IsCron:                 true,
			Memo:                   map[string][]byte{"memo": []byte("m")},
			SearchAttributes:       map[string][]byte{"sa": []byte("s")},
			HistorySize:            1234,
			BranchToken:            []byte("branch"),
		},
		VersionHistories: testVersionHistories(t),
		ActivityInfos:    map[int64]*persistence.InternalActivityInfo{1: {}},
		TimerInfos:       map[string]*persistence.TimerInfo{"t": {}},
		BufferedEvents:   []*persistence.DataBlob{{}},
		ChecksumData:     persistence.NewDataBlob([]byte("checksum"), constants.EncodingTypeJSON),
	}
}

// expectedSlimState is what reads return for a moved run: same execution info minus auto-reset points,
// empty pending-work maps, no checksum.
func expectedSlimState(t *testing.T, full *persistence.InternalWorkflowMutableState) *persistence.InternalWorkflowMutableState {
	info := *full.ExecutionInfo
	info.AutoResetPoints = nil
	return &persistence.InternalWorkflowMutableState{
		ExecutionInfo:       &info,
		VersionHistories:    full.VersionHistories,
		ActivityInfos:       map[int64]*persistence.InternalActivityInfo{},
		TimerInfos:          map[string]*persistence.TimerInfo{},
		ChildExecutionInfos: map[int64]*persistence.InternalChildExecutionInfo{},
		RequestCancelInfos:  map[int64]*persistence.RequestCancelInfo{},
		SignalInfos:         map[int64]*persistence.SignalInfo{},
		SemaphoreInfos:      map[int64]*persistence.SemaphoreInfo{},
		SignalRequestedIDs:  map[string]struct{}{},
	}
}

func TestBlobRoundTrip(t *testing.T) {
	parser := testParser(t)
	key := testKey()

	tests := map[string]struct {
		state   func() *persistence.InternalWorkflowMutableState
		wantErr bool
	}{
		"child cron run with retry": {
			state: func() *persistence.InternalWorkflowMutableState { return closedState(t, key) },
		},
		"continued as new, no parent": {
			state: func() *persistence.InternalWorkflowMutableState {
				s := closedState(t, key)
				s.ExecutionInfo.CloseStatus = persistence.WorkflowCloseStatusContinuedAsNew
				s.ExecutionInfo.ParentDomainID, s.ExecutionInfo.ParentWorkflowID, s.ExecutionInfo.ParentRunID = "", "", ""
				s.ExecutionInfo.InitiatedID = constants.EmptyEventID
				return s
			},
		},
		"legacy run without version histories stays hot": {
			state: func() *persistence.InternalWorkflowMutableState {
				s := closedState(t, key)
				s.VersionHistories = nil
				return s
			},
			wantErr: true,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			full := tc.state()
			blob, err := toBlob(parser, full)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)

			got, err := fromBlob(parser, persistence.NewPayloadSerializer(), key, blob)
			require.NoError(t, err)
			want := expectedSlimState(t, full)
			if want.ExecutionInfo.ParentDomainID == "" {
				want.ExecutionInfo.InitiatedID = 0 // the shared mapper only keeps it for child runs, same as SQL rows today
			}
			assert.Equal(t, want, got)
		})
	}
}

func TestBlobFuzz(t *testing.T) {
	parser := testParser(t)
	serializer := persistence.NewPayloadSerializer()
	key := testKey()
	versionHistories := testVersionHistories(t)

	testutils.RunMapperFuzzTest(t,
		func(info persistence.InternalWorkflowExecutionInfo) *persistence.DataBlob {
			blob, err := toBlob(parser, &persistence.InternalWorkflowMutableState{ExecutionInfo: &info, VersionHistories: versionHistories})
			require.NoError(t, err)
			return blob
		},
		func(blob *persistence.DataBlob) persistence.InternalWorkflowExecutionInfo {
			state, err := fromBlob(parser, serializer, key, blob)
			require.NoError(t, err)
			info := *state.ExecutionInfo
			// the shared mapper decodes times in local time
			for _, ts := range []*time.Time{&info.StartTimestamp, &info.LastUpdatedTimestamp, &info.ExpirationTime,
				&info.DecisionStartedTimestamp, &info.DecisionScheduledTimestamp, &info.DecisionOriginalScheduledTimestamp} {
				*ts = ts.UTC()
			}
			return info
		},
		// keep values inside what the existing executions blob can hold (SQL stores the same blob today)
		testutils.WithCustomFuncs(
			func(m *map[string][]byte, c fuzz.Continue) {
				*m = map[string][]byte{}
				for i := c.Intn(3); i > 0; i-- {
					(*m)[c.RandString()] = []byte(c.RandString())
				}
			},
			func(d *time.Duration, c fuzz.Continue) { *d = time.Duration(c.Int31()) * time.Second },
			func(i *int, c fuzz.Continue) { *i = int(c.Int31()) },
			func(b *persistence.DataBlob, c fuzz.Continue) {
				*b = persistence.DataBlob{Encoding: constants.EncodingTypeThriftRW, Data: []byte(c.RandString() + "x")}
			},
		),
		testutils.WithExcludedFields(
			// set from the row key and version histories on read
			"DomainID", "WorkflowID", "RunID", "NextEventID",
			// dropped on purpose, Describe rebuilds them
			"AutoResetPoints",
			// UUID-typed in the blob, so random strings can't round trip; covered by TestBlobRoundTrip
			"FirstExecutionRunID", "ParentDomainID", "ParentRunID", "ParentWorkflowID", "InitiatedID",
			// only kept when CancelRequested is set
			"CancelRequestID",
			// stored as an optional field that the shared mapper reads back differently when unset
			"IsCron",
		),
	)
}
