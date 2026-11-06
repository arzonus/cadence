// Package warmstorage moves closed runs out of the executions table into the slim closed_executions table.
//
// How it works: the close transfer task calls MoveToClosedExecutions once the run's visibility, parent and
// child work is done. The move copies a slim row into closed_executions and only then deletes the executions
// row. Reads of a run that isn't in executions fall back to closed_executions, and deletes remove both rows.
// History events never move.
//
// Opt-in: system.enableWarmStorage (per domain ID, default off) gates the move. Reads keep falling back after
// the flag is turned off, as long as this cluster has ever moved a run: a sentinel row records that, so Warm
// rows stay readable until retention deletes them. A cluster that never enabled the flag does nothing at
// startup and doesn't need the closed_executions table at all. Its only extra cost is one sentinel read per
// missed read or delete (at most one a minute while the table can't be read), which is also how a host whose
// flag lags learns that another host has moved runs.
package warmstorage

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/uber/cadence/common/constants"
	"github.com/uber/cadence/common/dynamicconfig/dynamicproperties"
	"github.com/uber/cadence/common/log"
	"github.com/uber/cadence/common/log/tag"
	"github.com/uber/cadence/common/metrics"
	"github.com/uber/cadence/common/persistence"
	"github.com/uber/cadence/common/persistence/serialization"
	"github.com/uber/cadence/common/types"
)

// sentinelKey marks a cluster that has moved at least one run. It is never deleted by Cadence.
var sentinelKey = persistence.ClosedExecutionKey{
	DomainID:   "00000000-0000-0000-0000-000000000000",
	WorkflowID: "cadence-warm-storage-sentinel",
	RunID:      "00000000-0000-0000-0000-000000000000",
}

// sentinelRetryInterval is how long a host leaves the sentinel alone after a read of it failed, most likely
// because the table doesn't exist.
const sentinelRetryInterval = time.Minute

type store struct {
	persistence.ExecutionStore
	closed     persistence.ClosedExecutionStore
	parser     serialization.Parser
	serializer persistence.PayloadSerializer
	enabled    dynamicproperties.BoolPropertyFnWithDomainIDFilter
	logger     log.Logger
	metrics    metrics.Client

	// inUse turns the read fallback on for every domain; it never goes back to false in a process
	inUse           atomic.Bool
	sentinelWritten atomic.Bool
	probeFailedAt   atomic.Int64 // unix nanos of the last failed sentinel read, 0 if none
}

// NewExecutionStore wraps an execution store with warm storage.
func NewExecutionStore(
	execution persistence.ExecutionStore,
	closed persistence.ClosedExecutionStore,
	parser serialization.Parser,
	dc *persistence.DynamicConfiguration,
	logger log.Logger,
	metricsClient metrics.Client,
) persistence.ExecutionStore {
	if metricsClient == nil {
		metricsClient = metrics.NewNoopMetricsClient()
	}
	return &store{
		ExecutionStore: execution,
		closed:         closed,
		parser:         parser,
		serializer:     persistence.NewPayloadSerializer(),
		enabled:        dc.EnableWarmStorage,
		logger:         logger,
		metrics:        metricsClient,
	}
}

func (s *store) GetWorkflowExecution(
	ctx context.Context,
	request *persistence.InternalGetWorkflowExecutionRequest,
) (*persistence.InternalGetWorkflowExecutionResponse, error) {
	resp, err := s.ExecutionStore.GetWorkflowExecution(ctx, request)
	if err == nil {
		s.markRestoredRun(ctx, request, resp)
		return resp, err
	}
	if !isNotExists(err) || !s.fallbackOn(ctx, request.DomainID) {
		return resp, err
	}
	key := persistence.ClosedExecutionKey{DomainID: request.DomainID, WorkflowID: request.Execution.WorkflowID, RunID: request.Execution.RunID}
	blob, closedErr := s.closed.GetClosedExecution(ctx, key)
	if isNotExists(closedErr) {
		return nil, err
	}
	if closedErr != nil {
		return nil, closedErr
	}
	state, closedErr := fromBlob(s.parser, s.serializer, key, blob)
	if closedErr != nil {
		return nil, closedErr
	}
	s.metrics.IncCounter(metrics.PersistenceGetWorkflowExecutionScope, metrics.WarmStorageFallbackHits)
	return &persistence.InternalGetWorkflowExecutionResponse{State: state}, nil
}

// markRestoredRun flags a closed run that has no reset points but still has its slim row: it was put back by a
// rehydrate, which loses the points, so Describe has to rebuild them. The slim row is only read for closed runs
// with empty points, which is rare.
func (s *store) markRestoredRun(ctx context.Context, request *persistence.InternalGetWorkflowExecutionRequest, resp *persistence.InternalGetWorkflowExecutionResponse) {
	info := resp.State.ExecutionInfo
	if info.State != persistence.WorkflowStateCompleted {
		return
	}
	// every manager write stores an empty list as a non-empty blob, so look at the decoded points
	if points, err := s.serializer.DeserializeResetPoints(info.AutoResetPoints); err != nil || len(points.Points) > 0 || !s.fallbackOn(ctx, request.DomainID) {
		return
	}
	key := persistence.ClosedExecutionKey{DomainID: request.DomainID, WorkflowID: request.Execution.WorkflowID, RunID: request.Execution.RunID}
	if _, err := s.closed.GetClosedExecution(ctx, key); err == nil {
		info.AutoResetPointsDropped = true
	}
}

func (s *store) IsWorkflowExecutionExists(
	ctx context.Context,
	request *persistence.IsWorkflowExecutionExistsRequest,
) (*persistence.IsWorkflowExecutionExistsResponse, error) {
	resp, err := s.ExecutionStore.IsWorkflowExecutionExists(ctx, request)
	if err != nil || resp.Exists || !s.fallbackOn(ctx, request.DomainID) {
		return resp, err
	}
	_, err = s.closed.GetClosedExecution(ctx, persistence.ClosedExecutionKey{DomainID: request.DomainID, WorkflowID: request.WorkflowID, RunID: request.RunID})
	if isNotExists(err) {
		return resp, nil
	}
	if err != nil {
		return nil, err
	}
	return &persistence.IsWorkflowExecutionExistsResponse{Exists: true}, nil
}

func (s *store) DeleteWorkflowExecution(ctx context.Context, request *persistence.DeleteWorkflowExecutionRequest) error {
	if err := s.ExecutionStore.DeleteWorkflowExecution(ctx, request); err != nil || !s.fallbackOn(ctx, request.DomainID) {
		return err
	}
	return s.closed.DeleteClosedExecution(ctx, persistence.ClosedExecutionKey{DomainID: request.DomainID, WorkflowID: request.WorkflowID, RunID: request.RunID})
}

// UpdateWorkflowExecution puts a Warm run back into executions when the update targets one, then retries once.
// Writers do reach closed runs: replication backfill, conflict resolution, refresh, and a child start that finds
// its child already closed all write to the run. Without the row they fail on a missing row.
func (s *store) UpdateWorkflowExecution(ctx context.Context, request *persistence.InternalUpdateWorkflowExecutionRequest) error {
	err := s.ExecutionStore.UpdateWorkflowExecution(ctx, request)
	if !isMissingRow(err) {
		return err
	}
	restored, restoreErr := s.restoreMovedRuns(ctx, request.ShardID, request.RangeID, request.CurrentTimeStamp, request.UpdateWorkflowMutation.ExecutionInfo)
	if restoreErr != nil || !restored {
		return firstError(restoreErr, err)
	}
	return s.ExecutionStore.UpdateWorkflowExecution(ctx, request)
}

// ConflictResolveWorkflowExecution does the same for the run being reset and the current run it updates.
func (s *store) ConflictResolveWorkflowExecution(ctx context.Context, request *persistence.InternalConflictResolveWorkflowExecutionRequest) error {
	err := s.ExecutionStore.ConflictResolveWorkflowExecution(ctx, request)
	if !isMissingRow(err) {
		return err
	}
	infos := []*persistence.InternalWorkflowExecutionInfo{request.ResetWorkflowSnapshot.ExecutionInfo}
	if request.CurrentWorkflowMutation != nil {
		infos = append(infos, request.CurrentWorkflowMutation.ExecutionInfo)
	}
	restored, restoreErr := s.restoreMovedRuns(ctx, request.ShardID, request.RangeID, request.CurrentTimeStamp, infos...)
	if restoreErr != nil || !restored {
		return firstError(restoreErr, err)
	}
	return s.ExecutionStore.ConflictResolveWorkflowExecution(ctx, request)
}

// restoreMovedRuns writes the executions row back for each run that has a slim row and no executions row.
// The slim row stays: reads prefer executions, and retention deletes both. A restored run moves again on its next
// close, otherwise it stays in executions until retention. Its reset points are gone from the row, so
// markRestoredRun flags it on read and Describe rebuilds them.
func (s *store) restoreMovedRuns(
	ctx context.Context,
	shardID persistence.ShardID,
	rangeID int64,
	now time.Time,
	infos ...*persistence.InternalWorkflowExecutionInfo,
) (restored bool, err error) {
	for _, info := range infos {
		if !s.fallbackOn(ctx, info.DomainID) {
			continue
		}
		key := persistence.ClosedExecutionKey{DomainID: info.DomainID, WorkflowID: info.WorkflowID, RunID: info.RunID}
		blob, err := s.closed.GetClosedExecution(ctx, key)
		if isNotExists(err) {
			continue
		}
		if err != nil {
			return false, err
		}
		execution := types.WorkflowExecution{WorkflowID: key.WorkflowID, RunID: key.RunID}
		if _, err := s.ExecutionStore.GetWorkflowExecution(ctx, &persistence.InternalGetWorkflowExecutionRequest{ShardID: shardID, DomainID: key.DomainID, Execution: execution}); err == nil {
			continue // the row is there, so the failure was a real condition failure
		} else if !isNotExists(err) {
			return false, err
		}
		state, err := fromBlob(s.parser, s.serializer, key, blob)
		if err != nil {
			return false, err
		}
		startVersion, lastWriteVersion, err := versionRange(s.serializer, state)
		if err != nil {
			return false, err
		}
		_, err = s.ExecutionStore.CreateWorkflowExecution(ctx, &persistence.InternalCreateWorkflowExecutionRequest{
			ShardID: shardID,
			RangeID: rangeID,
			Mode:    persistence.CreateWorkflowModeZombie, // writes the row and leaves current_executions alone
			NewWorkflowSnapshot: persistence.InternalWorkflowSnapshot{
				ExecutionInfo:    state.ExecutionInfo,
				VersionHistories: state.VersionHistories,
				StartVersion:     startVersion,
				LastWriteVersion: lastWriteVersion,
				RestoreClosedRun: true,
			},
			CurrentTimeStamp: now,
		})
		var alreadyThere *persistence.WorkflowExecutionAlreadyStartedError
		if err != nil && !errors.As(err, &alreadyThere) { // already there: another writer restored it first
			return false, err
		}
		s.metrics.IncCounter(metrics.PersistenceUpdateWorkflowExecutionScope, metrics.WarmStorageRehydrates)
		restored = true
	}
	return restored, nil
}

// isMissingRow reports what the stores return for a write whose row is gone: SQL says not found, Cassandra
// fails the next-event-ID condition.
func isMissingRow(err error) bool {
	var conditionFailed *persistence.ConditionFailedError
	return isNotExists(err) || errors.As(err, &conditionFailed)
}

func firstError(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// MoveToClosedExecutions fails open: any problem leaves the run in executions, exactly as without warm storage,
// so the close task never gets stuck on it. The caller checks the flag.
func (s *store) MoveToClosedExecutions(ctx context.Context, request *persistence.DeleteWorkflowExecutionRequest) error {
	resp, err := s.ExecutionStore.GetWorkflowExecution(ctx, &persistence.InternalGetWorkflowExecutionRequest{
		ShardID:   request.ShardID,
		DomainID:  request.DomainID,
		Execution: types.WorkflowExecution{WorkflowID: request.WorkflowID, RunID: request.RunID},
	})
	if isNotExists(err) {
		return nil // already moved, e.g. the close task ran twice
	}
	if err != nil {
		return s.moveFailed(request, err)
	}
	if resp.State.ExecutionInfo.State != persistence.WorkflowStateCompleted {
		return nil // e.g. standby hasn't applied the close yet; the run just stays hot
	}
	if len(resp.State.ActivityInfos) > 0 || len(resp.State.ChildExecutionInfos) > 0 {
		// a run that timed out or was terminated with work in flight still lists it in Describe; the slim row can't
		s.metrics.IncCounter(metrics.PersistenceMoveToClosedExecutionsScope, metrics.WarmStorageMovesSkipped)
		return nil
	}
	key := persistence.ClosedExecutionKey{DomainID: request.DomainID, WorkflowID: request.WorkflowID, RunID: request.RunID}
	blob, err := toBlob(s.parser, resp.State)
	if err != nil {
		return s.moveFailed(request, err)
	}
	// read the slim row back before anything is deleted: a row that reads wrong would lose the run
	slim, err := fromBlob(s.parser, s.serializer, key, blob)
	if err != nil {
		return s.moveFailed(request, err)
	}
	if got, want := slim.ExecutionInfo.NextEventID, resp.State.ExecutionInfo.NextEventID; got != want {
		return s.moveFailed(request, fmt.Errorf("next event ID from version histories is %d, row has %d", got, want))
	}
	if err := s.writeSentinel(ctx); err != nil {
		return s.moveFailed(request, err)
	}
	// slim row first: if the delete never happens both rows exist, and reads still prefer executions
	if err := s.closed.InsertClosedExecution(ctx, key, blob); err != nil {
		return s.moveFailed(request, err)
	}
	// delete only the row that was copied: a writer that got in since the read (another host, after a shard move)
	// has made the executions row the truth, and the slim row is stale
	del := *request
	del.NextEventID = &resp.State.ExecutionInfo.NextEventID
	err = s.ExecutionStore.DeleteWorkflowExecution(ctx, &del)
	var conditionFailed *persistence.ConditionFailedError
	switch {
	case isNotExists(err):
		return nil // moved or deleted by someone else in the meantime
	case errors.As(err, &conditionFailed):
		s.metrics.IncCounter(metrics.PersistenceMoveToClosedExecutionsScope, metrics.WarmStorageMoveConflicts)
		if err := s.closed.DeleteClosedExecution(ctx, key); err != nil {
			return s.moveFailed(request, err)
		}
		return nil
	case err != nil:
		return s.moveFailed(request, err)
	}
	s.metrics.IncCounter(metrics.PersistenceMoveToClosedExecutionsScope, metrics.WarmStorageMoves)
	return nil
}

func (s *store) Close() {
	s.closed.Close()
	s.ExecutionStore.Close()
}

func (s *store) fallbackOn(ctx context.Context, domainID string) bool {
	if s.enabled(domainID) {
		s.inUse.Store(true)
		return true
	}
	if !s.inUse.Load() {
		s.probeSentinel(ctx)
	}
	return s.inUse.Load()
}

// probeSentinel reads the sentinel on every call while it isn't known to exist, so a delete never skips the
// closed row because another host has moved runs this one hasn't heard of. A failed read (most likely no table)
// isn't repeated for sentinelRetryInterval.
func (s *store) probeSentinel(ctx context.Context) {
	if failed := s.probeFailedAt.Load(); failed != 0 && time.Since(time.Unix(0, failed)) < sentinelRetryInterval {
		return
	}
	_, err := s.closed.GetClosedExecution(ctx, sentinelKey)
	switch {
	case err == nil:
		s.probeFailedAt.Store(0)
		s.inUse.Store(true)
		s.sentinelWritten.Store(true)
	case isNotExists(err):
		s.probeFailedAt.Store(0)
	default:
		// most likely the table isn't there: fine until someone turns the flag on
		s.probeFailedAt.Store(time.Now().UnixNano())
		s.logger.Info("warm storage is unavailable, closed_executions can't be read", tag.Error(err))
	}
}

func (s *store) writeSentinel(ctx context.Context) error {
	if s.sentinelWritten.Load() {
		return nil
	}
	if err := s.closed.InsertClosedExecution(ctx, sentinelKey, persistence.NewDataBlob([]byte{0}, constants.EncodingTypeEmpty)); err != nil {
		return err
	}
	s.inUse.Store(true)
	s.sentinelWritten.Store(true)
	return nil
}

func (s *store) moveFailed(request *persistence.DeleteWorkflowExecutionRequest, err error) error {
	s.metrics.IncCounter(metrics.PersistenceMoveToClosedExecutionsScope, metrics.WarmStorageMoveFailures)
	s.logger.Error("warm storage move failed, run stays in executions",
		tag.WorkflowDomainID(request.DomainID),
		tag.WorkflowID(request.WorkflowID),
		tag.WorkflowRunID(request.RunID),
		tag.Error(err),
	)
	return nil
}

func isNotExists(err error) bool {
	var notExists *types.EntityNotExistsError
	return errors.As(err, &notExists)
}
