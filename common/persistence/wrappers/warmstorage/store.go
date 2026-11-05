// Package warmstorage moves closed runs out of the executions table into the slim closed_executions table.
//
// How it works: the close transfer task calls MoveToClosedExecutions once the run's visibility, parent and
// child work is done. The move copies a slim row into closed_executions and only then deletes the executions
// row. Reads of a run that isn't in executions fall back to closed_executions, and deletes remove both rows.
// History events never move.
//
// Opt-in: system.enableWarmStorage (per domain ID, default off) gates the move. Reads keep falling back after
// the flag is turned off, as long as this cluster has ever moved a run: a sentinel row records that, so Warm
// rows stay readable until retention deletes them. A cluster that never enabled the flag issues no extra
// queries besides one sentinel read at startup, and doesn't need the closed_executions table at all.
package warmstorage

import (
	"context"
	"errors"
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

const sentinelProbeTimeout = 5 * time.Second

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
	s := &store{
		ExecutionStore: execution,
		closed:         closed,
		parser:         parser,
		serializer:     persistence.NewPayloadSerializer(),
		enabled:        dc.EnableWarmStorage,
		logger:         logger,
		metrics:        metricsClient,
	}
	ctx, cancel := context.WithTimeout(context.Background(), sentinelProbeTimeout)
	defer cancel()
	if _, err := closed.GetClosedExecution(ctx, sentinelKey); err == nil {
		s.inUse.Store(true)
		s.sentinelWritten.Store(true)
	} else if !isNotExists(err) {
		// most likely the table isn't there: fine until someone turns the flag on
		logger.Info("warm storage is unavailable, closed_executions can't be read", tag.Error(err))
	}
	return s
}

func (s *store) GetWorkflowExecution(
	ctx context.Context,
	request *persistence.InternalGetWorkflowExecutionRequest,
) (*persistence.InternalGetWorkflowExecutionResponse, error) {
	resp, err := s.ExecutionStore.GetWorkflowExecution(ctx, request)
	if !isNotExists(err) || !s.fallbackOn(request.DomainID) {
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

func (s *store) IsWorkflowExecutionExists(
	ctx context.Context,
	request *persistence.IsWorkflowExecutionExistsRequest,
) (*persistence.IsWorkflowExecutionExistsResponse, error) {
	resp, err := s.ExecutionStore.IsWorkflowExecutionExists(ctx, request)
	if err != nil || resp.Exists || !s.fallbackOn(request.DomainID) {
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
	if err := s.ExecutionStore.DeleteWorkflowExecution(ctx, request); err != nil || !s.fallbackOn(request.DomainID) {
		return err
	}
	return s.closed.DeleteClosedExecution(ctx, persistence.ClosedExecutionKey{DomainID: request.DomainID, WorkflowID: request.WorkflowID, RunID: request.RunID})
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
		return s.moveFailed(request, errors.New("run is not closed"))
	}
	blob, err := toBlob(s.parser, resp.State)
	if err != nil {
		return s.moveFailed(request, err)
	}
	if err := s.writeSentinel(ctx); err != nil {
		return s.moveFailed(request, err)
	}
	key := persistence.ClosedExecutionKey{DomainID: request.DomainID, WorkflowID: request.WorkflowID, RunID: request.RunID}
	// slim row first: if the delete never happens both rows exist, and reads still prefer executions
	if err := s.closed.InsertClosedExecution(ctx, key, blob); err != nil {
		return s.moveFailed(request, err)
	}
	if err := s.ExecutionStore.DeleteWorkflowExecution(ctx, request); err != nil {
		return s.moveFailed(request, err)
	}
	s.metrics.IncCounter(metrics.PersistenceMoveToClosedExecutionsScope, metrics.WarmStorageMoves)
	return nil
}

func (s *store) Close() {
	s.closed.Close()
	s.ExecutionStore.Close()
}

func (s *store) fallbackOn(domainID string) bool {
	if s.enabled(domainID) {
		s.inUse.Store(true)
		return true
	}
	return s.inUse.Load()
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
