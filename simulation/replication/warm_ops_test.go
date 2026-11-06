package replication

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/uber/cadence/common/types"
	simTypes "github.com/uber/cadence/simulation/replication/types"
)

// runExecution returns the workflow execution an op targets: the run stored under op.RunIDKey, or the latest run.
func runExecution(op *simTypes.Operation, sim *simTypes.ReplicationSimulation) (*types.WorkflowExecution, error) {
	exec := &types.WorkflowExecution{WorkflowID: op.WorkflowID}
	if op.RunIDKey != "" {
		runID, err := sim.GetRunID(op.RunIDKey)
		if err != nil {
			return nil, err
		}
		exec.RunID = runID
	}
	return exec, nil
}

func terminateWorkflow(t *testing.T, op *simTypes.Operation, simCfg *simTypes.ReplicationSimulationConfig, sim *simTypes.ReplicationSimulation) error {
	t.Helper()
	exec, err := runExecution(op, sim)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	simTypes.Logf(t, "Terminating workflow %s on cluster %s", op.WorkflowID, op.Cluster)
	return simCfg.MustGetFrontendClient(t, op.Cluster).TerminateWorkflowExecution(ctx, &types.TerminateWorkflowExecutionRequest{
		Domain:            op.Domain,
		WorkflowExecution: exec,
		Reason:            "replication simulation",
		Identity:          "replication-simulator",
	})
}

// workflowFingerprint is what must be identical on both clusters once replication has caught up. It leaves out the describe StartTime: a reset run gets a different one per cluster (flag off too).
func workflowFingerprint(t *testing.T, simCfg *simTypes.ReplicationSimulationConfig, cluster, domain string, exec *types.WorkflowExecution) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	desc, err := simCfg.MustGetFrontendClient(t, cluster).DescribeWorkflowExecution(ctx, &types.DescribeWorkflowExecutionRequest{Domain: domain, Execution: exec})
	if err != nil {
		return "", fmt.Errorf("describe on %s: %w", cluster, err)
	}
	info := desc.WorkflowExecutionInfo
	history, err := getAllHistory(t, simCfg, cluster, domain, exec.WorkflowID, info.Execution.RunID)
	if err != nil {
		return "", fmt.Errorf("history on %s: %w", cluster, err)
	}
	pendingAct, pendingChild := len(desc.PendingActivities), len(desc.PendingChildren)
	if info.CloseStatus != nil {
		// A closed run served from a cached mutable state (hot) lists its pending activities, one served from closed_executions (warm) does not.
		pendingAct, pendingChild = 0, 0
	}
	b, err := json.Marshal(struct {
		Execution     *types.WorkflowExecution
		Type          *types.WorkflowType
		CloseTime     *int64
		CloseStatus   *types.WorkflowExecutionCloseStatus
		HistoryLength int64
		PendingAct    int
		PendingChild  int
		History       []types.HistoryEvent
	}{info.Execution, info.Type, info.CloseTime, info.CloseStatus, info.HistoryLength, pendingAct, pendingChild, history})
	return string(b), err
}

// compareClusters waits until Describe and the full GetHistory of a run are identical on both clusters.
func compareClusters(t *testing.T, op *simTypes.Operation, simCfg *simTypes.ReplicationSimulationConfig, sim *simTypes.ReplicationSimulation) error {
	t.Helper()
	exec, err := runExecution(op, sim)
	if err != nil {
		return err
	}
	var lastErr error
	for deadline := time.Now().Add(60 * time.Second); time.Now().Before(deadline); time.Sleep(2 * time.Second) {
		var a, b string
		if a, lastErr = workflowFingerprint(t, simCfg, op.SourceCluster, op.Domain, exec); lastErr != nil {
			continue
		}
		if b, lastErr = workflowFingerprint(t, simCfg, op.TargetCluster, op.Domain, exec); lastErr != nil {
			continue
		}
		if a == b {
			simTypes.Logf(t, "SIM-COMPARE-OK %s identical on %s and %s (%d bytes)", op.WorkflowID, op.SourceCluster, op.TargetCluster, len(a))
			return nil
		}
		lastErr = fmt.Errorf("%s differs between %s and %s.\n%s: %s\n%s: %s", op.WorkflowID, op.SourceCluster, op.TargetCluster, op.SourceCluster, a, op.TargetCluster, b)
	}
	return lastErr
}

// validateRawHistory reads a run through the admin raw history API, which is what the history resender calls on the active cluster.
func validateRawHistory(t *testing.T, op *simTypes.Operation, simCfg *simTypes.ReplicationSimulationConfig, sim *simTypes.ReplicationSimulation) error {
	t.Helper()
	exec, err := runExecution(op, sim)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if exec.RunID == "" { // the admin API needs a run ID
		desc, err := simCfg.MustGetFrontendClient(t, op.Cluster).DescribeWorkflowExecution(ctx, &types.DescribeWorkflowExecutionRequest{Domain: op.Domain, Execution: exec})
		if err != nil {
			return err
		}
		exec = desc.WorkflowExecutionInfo.Execution
	}
	resp, err := simCfg.Clusters[op.Cluster].AdminClient.GetWorkflowExecutionRawHistoryV2(ctx, &types.GetWorkflowExecutionRawHistoryV2Request{
		Domain:          op.Domain,
		Execution:       exec,
		MaximumPageSize: 100,
	})
	if err != nil {
		return fmt.Errorf("raw history of %s on %s: %w", op.WorkflowID, op.Cluster, err)
	}
	if len(resp.HistoryBatches) == 0 || resp.VersionHistory == nil || len(resp.VersionHistory.Items) == 0 {
		return fmt.Errorf("raw history of %s on %s is empty", op.WorkflowID, op.Cluster)
	}
	simTypes.Logf(t, "SIM-RAWHISTORY-OK %s on %s: %d batches, version history %+v", op.WorkflowID, op.Cluster, len(resp.HistoryBatches), resp.VersionHistory.Items)
	return nil
}

func validateDLQEmpty(t *testing.T, op *simTypes.Operation, simCfg *simTypes.ReplicationSimulationConfig) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, err := simCfg.Clusters[op.Cluster].AdminClient.CountDLQMessages(ctx, &types.CountDLQMessagesRequest{ForceFetch: true})
	if err != nil {
		return err
	}
	var total int64
	for _, n := range resp.History {
		total += n
	}
	if total+resp.Domain != 0 {
		return fmt.Errorf("replication DLQ on %s is not empty: history %v, domain %d", op.Cluster, resp.History, resp.Domain)
	}
	simTypes.Logf(t, "SIM-DLQ-EMPTY on %s", op.Cluster)
	return nil
}
