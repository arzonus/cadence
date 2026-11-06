package host

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gocql/gocql"
	"github.com/pborman/uuid"
	"github.com/stretchr/testify/suite"
	"github.com/uber-go/tally"

	"github.com/uber/cadence/common"
	"github.com/uber/cadence/common/config"
	"github.com/uber/cadence/common/metrics"
	"github.com/uber/cadence/common/persistence"
	"github.com/uber/cadence/common/persistence/client"
	"github.com/uber/cadence/common/types"
	"github.com/uber/cadence/environment"
)

// The warm storage tests are methods of IntegrationSuite but only run through
// TestWarmStorageIntegrationSuite, which turns the mode on through a temp dynamic config file
// and opens a raw Cassandra session. In the regular IntegrationSuite they skip.
var (
	warmDCPath  string
	warmSession *gocql.Session
)

func TestWarmStorageIntegrationSuite(t *testing.T) {
	flag.Parse()
	if TestFlags.PersistenceType != config.StoreTypeCassandra {
		t.Skip("warm storage host tests need Cassandra")
	}
	clusterConfig, err := GetTestClusterConfig("testdata/integration_test_cluster.yaml")
	if err != nil {
		t.Fatal(err)
	}
	base, err := os.ReadFile(clusterConfig.DynamicClientConfig.Filepath)
	if err != nil {
		t.Fatal(err)
	}
	warmDCPath = filepath.Join(t.TempDir(), "dc.yaml")
	t.Cleanup(func() { warmDCPath = "" })
	setWarmFlag(t, string(base), true)
	clusterConfig.DynamicClientConfig.Filepath = warmDCPath
	clusterConfig.DynamicClientConfig.PollInterval = 5 * time.Second

	testCluster := NewTestPersistenceConfig(t)
	port, err := environment.GetCassandraPort()
	if err != nil {
		t.Fatal(err)
	}
	cl := gocql.NewCluster(environment.GetCassandraAddress())
	cl.Port = port
	cl.Keyspace = testCluster.DataStores[testCluster.DefaultStore].NoSQL.Keyspace
	s := new(IntegrationSuite)
	s.IntegrationBase = NewIntegrationBase(IntegrationBaseParams{PersistenceConfig: testCluster, TestClusterConfig: clusterConfig})
	// The keyspace is created by NewTestPersistenceConfig/setupSuite, so open the session lazily.
	t.Cleanup(func() {
		if warmSession != nil {
			warmSession.Close()
			warmSession = nil
		}
	})
	warmClusterCfg = cl
	suite.Run(t, s)
}

var warmClusterCfg *gocql.ClusterConfig

var warmBaseDC string

func warmModeOf(on bool) string {
	if on {
		return "on"
	}
	return "off"
}

func setWarmFlag(t *testing.T, base string, on bool) {
	setWarmMode(t, base, warmModeOf(on))
}

func setWarmMode(t *testing.T, base string, mode string) {
	if base != "" {
		warmBaseDC = base
	}
	// quoted: a bare on is a YAML boolean
	content := warmBaseDC + "\nsystem.warmStorageMode:\n- value: \"" + mode + "\"\n  constraints: {}\n"
	if err := os.WriteFile(warmDCPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (s *IntegrationSuite) warmSkip() {
	if warmDCPath == "" {
		s.T().Skip("only run by TestWarmStorageIntegrationSuite")
	}
}

func (s *IntegrationSuite) warmDomainID() string {
	ctx, cancel := createContext()
	defer cancel()
	resp, err := s.Engine.DescribeDomain(ctx, &types.DescribeDomainRequest{Name: &s.DomainName})
	s.NoError(err)
	return resp.DomainInfo.GetUUID()
}

// warmRows returns how many rows the run has in executions (hot) and closed_executions (warm).
func (s *IntegrationSuite) warmRows(id, runID string) (hot, warm int) {
	if warmSession == nil {
		var err error
		warmSession, err = warmClusterCfg.CreateSession()
		s.NoError(err)
	}
	shard := common.WorkflowIDToHistoryShard(id, s.TestClusterConfig.HistoryConfig.NumHistoryShards)
	domainID := s.warmDomainID()
	s.NoError(warmSession.Query(`SELECT count(*) FROM executions WHERE shard_id=? AND type=1 AND domain_id=? AND workflow_id=? AND run_id=?`,
		shard, domainID, id, runID).Scan(&hot))
	s.NoError(warmSession.Query(`SELECT count(*) FROM closed_executions WHERE domain_id=? AND workflow_id=? AND run_id=?`,
		domainID, id, runID).Scan(&warm))
	return hot, warm
}

func (s *IntegrationSuite) warmWaitMoved(id, runID string) {
	var hot, warm int
	for i := 0; i < 60; i++ {
		if hot, warm = s.warmRows(id, runID); hot == 0 && warm == 1 {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	s.Failf("run not moved", "wf %s run %s: executions rows=%d closed_executions rows=%d", id, runID, hot, warm)
}

func (s *IntegrationSuite) warmWaitHot(id, runID string) {
	time.Sleep(3 * time.Second) // moves are async; give a would-be move time to happen
	hot, warm := s.warmRows(id, runID)
	s.Equal([]int{1, 0}, []int{hot, warm}, "run must stay hot")
}

func (s *IntegrationSuite) warmStart(id string, mut func(*types.StartWorkflowExecutionRequest)) string {
	req := &types.StartWorkflowExecutionRequest{
		RequestID:                           uuid.New(),
		Domain:                              s.DomainName,
		WorkflowID:                          id,
		WorkflowType:                        &types.WorkflowType{Name: "warm-type"},
		TaskList:                            &types.TaskList{Name: id},
		Memo:                                &types.Memo{Fields: map[string][]byte{"k": []byte(`"v"`)}},
		ExecutionStartToCloseTimeoutSeconds: common.Int32Ptr(100),
		TaskStartToCloseTimeoutSeconds:      common.Int32Ptr(10),
		Identity:                            "warm",
	}
	if mut != nil {
		mut(req)
	}
	ctx, cancel := createContext()
	defer cancel()
	resp, err := s.Engine.StartWorkflowExecution(ctx, req)
	s.NoError(err)
	return resp.RunID
}

// warmDecide polls one decision task of the workflow's task list and answers it.
func (s *IntegrationSuite) warmDecide(tl, checksum string, decisions ...*types.Decision) {
	ctx, cancel := createContext()
	defer cancel()
	poll, err := s.Engine.PollForDecisionTask(ctx, &types.PollForDecisionTaskRequest{
		Domain: s.DomainName, TaskList: &types.TaskList{Name: tl}, Identity: "warm",
	})
	s.NoError(err)
	s.NotEmpty(poll.TaskToken, "no decision task")
	_, err = s.Engine.RespondDecisionTaskCompleted(ctx, &types.RespondDecisionTaskCompletedRequest{
		TaskToken: poll.TaskToken, Decisions: decisions, Identity: "warm", BinaryChecksum: checksum,
	})
	s.NoError(err)
}

func completeD() *types.Decision {
	return &types.Decision{
		DecisionType: types.DecisionTypeCompleteWorkflowExecution.Ptr(),
		CompleteWorkflowExecutionDecisionAttributes: &types.CompleteWorkflowExecutionDecisionAttributes{Result: []byte("done")},
	}
}

func failD() *types.Decision {
	return &types.Decision{
		DecisionType:                            types.DecisionTypeFailWorkflowExecution.Ptr(),
		FailWorkflowExecutionDecisionAttributes: &types.FailWorkflowExecutionDecisionAttributes{Reason: common.StringPtr("warm-fail")},
	}
}

func (s *IntegrationSuite) warmDescribe(id, runID string) *types.DescribeWorkflowExecutionResponse {
	ctx, cancel := createContext()
	defer cancel()
	resp, err := s.Engine.DescribeWorkflowExecution(ctx, &types.DescribeWorkflowExecutionRequest{
		Domain: s.DomainName, Execution: &types.WorkflowExecution{WorkflowID: id, RunID: runID},
	})
	s.NoError(err)
	return resp
}

func (s *IntegrationSuite) warmHistory(id, runID string) []*types.HistoryEvent {
	ctx, cancel := createContext()
	defer cancel()
	resp, err := s.Engine.GetWorkflowExecutionHistory(ctx, &types.GetWorkflowExecutionHistoryRequest{
		Domain: s.DomainName, Execution: &types.WorkflowExecution{WorkflowID: id, RunID: runID},
	})
	s.NoError(err)
	return resp.History.Events
}

// warmCurrentRun returns the run ID of the workflow's current run (current_executions pointer).
func (s *IntegrationSuite) warmCurrentRun(id string) string {
	return s.warmDescribe(id, "").WorkflowExecutionInfo.Execution.RunID
}

// 1, 2, 3, 4: every close status moves, and Describe / History / Signal / Terminate behave on the warm run.
func (s *IntegrationSuite) TestWarmStorage_CloseStatuses_SLOW() {
	s.warmSkip()
	cases := []struct {
		name   string
		status types.WorkflowExecutionCloseStatus
		closer func(id string) string // returns the run that closes
	}{
		{"completed", types.WorkflowExecutionCloseStatusCompleted, func(id string) string {
			run := s.warmStart(id, nil)
			s.warmDecide(id, "bin-1", completeD())
			return run
		}},
		{"failed", types.WorkflowExecutionCloseStatusFailed, func(id string) string {
			run := s.warmStart(id, nil)
			s.warmDecide(id, "bin-1", failD())
			return run
		}},
		{"canceled", types.WorkflowExecutionCloseStatusCanceled, func(id string) string {
			run := s.warmStart(id, nil)
			ctx, cancel := createContext()
			defer cancel()
			s.NoError(s.Engine.RequestCancelWorkflowExecution(ctx, &types.RequestCancelWorkflowExecutionRequest{
				Domain: s.DomainName, WorkflowExecution: &types.WorkflowExecution{WorkflowID: id, RunID: run}, Identity: "warm"}))
			s.warmDecide(id, "bin-1", &types.Decision{
				DecisionType: types.DecisionTypeCancelWorkflowExecution.Ptr(),
				CancelWorkflowExecutionDecisionAttributes: &types.CancelWorkflowExecutionDecisionAttributes{},
			})
			return run
		}},
		{"terminated", types.WorkflowExecutionCloseStatusTerminated, func(id string) string {
			run := s.warmStart(id, nil)
			ctx, cancel := createContext()
			defer cancel()
			s.NoError(s.Engine.TerminateWorkflowExecution(ctx, &types.TerminateWorkflowExecutionRequest{
				Domain: s.DomainName, WorkflowExecution: &types.WorkflowExecution{WorkflowID: id, RunID: run}, Reason: "warm"}))
			return run
		}},
		{"timedout", types.WorkflowExecutionCloseStatusTimedOut, func(id string) string {
			return s.warmStart(id, func(r *types.StartWorkflowExecutionRequest) {
				r.ExecutionStartToCloseTimeoutSeconds = common.Int32Ptr(1)
			})
		}},
		{"continuedasnew", types.WorkflowExecutionCloseStatusContinuedAsNew, func(id string) string {
			run := s.warmStart(id, nil)
			s.warmDecide(id, "bin-1", &types.Decision{
				DecisionType: types.DecisionTypeContinueAsNewWorkflowExecution.Ptr(),
				ContinueAsNewWorkflowExecutionDecisionAttributes: &types.ContinueAsNewWorkflowExecutionDecisionAttributes{
					WorkflowType:                        &types.WorkflowType{Name: "warm-type"},
					TaskList:                            &types.TaskList{Name: id},
					ExecutionStartToCloseTimeoutSeconds: common.Int32Ptr(100),
					TaskStartToCloseTimeoutSeconds:      common.Int32Ptr(10),
				}})
			return run
		}},
	}
	for _, tc := range cases {
		s.Run(tc.name, func() {
			id := "warm-close-" + tc.name + "-" + uuid.New()
			run := tc.closer(id)
			s.warmWaitMoved(id, run)

			desc := s.warmDescribe(id, run)
			s.Equal(tc.status, *desc.WorkflowExecutionInfo.CloseStatus)
			if tc.name != "continuedasnew" {
				s.Equal(`"v"`, string(desc.WorkflowExecutionInfo.Memo.Fields["k"]))
			}
			events := s.warmHistory(id, run)
			s.Equal(types.EventTypeWorkflowExecutionStarted, *events[0].EventType)
			s.Equal(desc.WorkflowExecutionInfo.HistoryLength, int64(len(events)))

			// 4: writes to a warm run
			ctx, cancel := createContext()
			defer cancel()
			exec := &types.WorkflowExecution{WorkflowID: id, RunID: run}
			sigErr, termErr := s.warmLateWrites(id, run)
			s.IsType(sigErr, termErr)
			s.True(isCompletedErr(sigErr), "signal: %v", sigErr)

			if tc.name == "completed" { // persistence-level fallback read
				mgr := s.warmExecMgr()
				defer mgr.Close()
				resp, err := mgr.GetWorkflowExecution(ctx, &persistence.GetWorkflowExecutionRequest{
					DomainID: s.warmDomainID(), Execution: *exec,
					ShardID: common.IntPtr(common.WorkflowIDToHistoryShard(id, s.TestClusterConfig.HistoryConfig.NumHistoryShards))})
				s.NoError(err)
				s.Equal(persistence.WorkflowCloseStatusCompleted, resp.State.ExecutionInfo.CloseStatus)
			}
		})
	}
}

func isCompletedErr(err error) bool {
	switch err.(type) {
	case *types.WorkflowExecutionAlreadyCompletedError, *types.EntityNotExistsError:
		return true
	}
	return false
}

func (s *IntegrationSuite) warmLateWrites(id, run string) (sigErr, termErr error) {
	ctx, cancel := createContext()
	defer cancel()
	exec := &types.WorkflowExecution{WorkflowID: id, RunID: run}
	sigErr = s.Engine.SignalWorkflowExecution(ctx, &types.SignalWorkflowExecutionRequest{
		Domain: s.DomainName, WorkflowExecution: exec, SignalName: "sig", Identity: "warm"})
	termErr = s.Engine.TerminateWorkflowExecution(ctx, &types.TerminateWorkflowExecutionRequest{
		Domain: s.DomainName, WorkflowExecution: exec, Reason: "late"})
	return sigErr, termErr
}

func (s *IntegrationSuite) warmExecMgr() persistence.ExecutionManager {
	return s.warmExecMgrMode("on", metrics.NewNoopMetricsClient())
}

// warmExecMgrMode is an execution manager of the test's own, in the given mode, so its metrics can be read.
func (s *IntegrationSuite) warmExecMgrMode(mode string, metricsClient metrics.Client) persistence.ExecutionManager {
	dc := s.TestCluster.testBase.DynamicConfiguration
	dc.WarmStorageMode = func(string) string { return mode }
	pConfig := s.PersistenceConfig
	mgr, err := client.NewFactory(&pConfig, func() float64 { return 1000 },
		s.TestCluster.testBase.ClusterMetadata.GetCurrentClusterName(),
		metricsClient, s.Logger, &dc).NewExecutionManager()
	s.NoError(err)
	return mgr
}

// 2, 5, 6: Describe equality (reset points), reset of a warm run, workflow ID reuse.
func (s *IntegrationSuite) TestWarmStorage_DescribeResetReuse_SLOW() {
	s.warmSkip()
	id := "warm-reset-" + uuid.New()
	run := s.warmStart(id, nil)
	s.warmDecide(id, "bin-1") // completes decision task with no decisions; records a reset point
	before := s.warmDescribe(id, run).WorkflowExecutionInfo
	s.NotEmpty(before.AutoResetPoints.Points)
	ctx, cancel := createContext()
	defer cancel()
	exec := &types.WorkflowExecution{WorkflowID: id, RunID: run}
	s.NoError(s.Engine.SignalWorkflowExecution(ctx, &types.SignalWorkflowExecutionRequest{
		Domain: s.DomainName, WorkflowExecution: exec, SignalName: "go", Identity: "warm"}))
	s.warmDecide(id, "bin-1", completeD())
	s.warmWaitMoved(id, run)

	// closing the shard evicts the history cache, so Describe reads the slim row
	shard := int32(common.WorkflowIDToHistoryShard(id, s.TestClusterConfig.HistoryConfig.NumHistoryShards))
	s.NoError(s.HistoryClient.CloseShard(ctx, &types.CloseShardRequest{ShardID: shard}))
	time.Sleep(time.Second)
	after := s.warmDescribe(id, run).WorkflowExecutionInfo
	// CreatedTimeNano is replay time by design (see describe_rebuild_reset_points.go), so compare the rest
	s.Len(after.AutoResetPoints.Points, len(before.AutoResetPoints.Points))
	for i, p := range after.AutoResetPoints.Points {
		s.NotNil(p.CreatedTimeNano)
		p.CreatedTimeNano = before.AutoResetPoints.Points[i].CreatedTimeNano
	}
	s.Equal(before.AutoResetPoints, after.AutoResetPoints, "reset points rebuilt from history must equal the pre-close ones")
	s.Equal(before.Memo, after.Memo)
	s.Equal(types.WorkflowExecutionCloseStatusCompleted, *after.CloseStatus)

	// 5: reset the warm run to the first decision-task-completed event
	var firstCompleted int64
	for _, e := range s.warmHistory(id, run) {
		if *e.EventType == types.EventTypeDecisionTaskCompleted {
			firstCompleted = e.ID
			break
		}
	}
	reset, err := s.Engine.ResetWorkflowExecution(ctx, &types.ResetWorkflowExecutionRequest{
		Domain: s.DomainName, WorkflowExecution: exec, Reason: "warm", DecisionFinishEventID: firstCompleted, RequestID: uuid.New()})
	s.NoError(err)
	newRun := reset.RunID
	s.NotEqual(run, newRun)
	s.warmDecide(id, "bin-1", completeD())
	s.warmWaitMoved(id, newRun)
	s.Equal(types.WorkflowExecutionCloseStatusCompleted, *s.warmDescribe(id, newRun).WorkflowExecutionInfo.CloseStatus)

	// 6: workflow ID reuse after the move
	reuse := func(p types.WorkflowIDReusePolicy) error {
		_, err := s.Engine.StartWorkflowExecution(ctx, &types.StartWorkflowExecutionRequest{
			RequestID: uuid.New(), Domain: s.DomainName, WorkflowID: id,
			WorkflowType: &types.WorkflowType{Name: "warm-type"}, TaskList: &types.TaskList{Name: id},
			ExecutionStartToCloseTimeoutSeconds: common.Int32Ptr(100), TaskStartToCloseTimeoutSeconds: common.Int32Ptr(10),
			WorkflowIDReusePolicy: &p, Identity: "warm"})
		return err
	}
	err = reuse(types.WorkflowIDReusePolicyRejectDuplicate)
	s.IsType(&types.WorkflowExecutionAlreadyStartedError{}, err, "reject: %v", err)
	s.NoError(reuse(types.WorkflowIDReusePolicyAllowDuplicate))
}

// 7: continue-as-new, cron and retry chains.
func (s *IntegrationSuite) TestWarmStorage_Chains_SLOW() {
	s.warmSkip()
	can := func(id string) *types.Decision {
		return &types.Decision{
			DecisionType: types.DecisionTypeContinueAsNewWorkflowExecution.Ptr(),
			ContinueAsNewWorkflowExecutionDecisionAttributes: &types.ContinueAsNewWorkflowExecutionDecisionAttributes{
				WorkflowType: &types.WorkflowType{Name: "warm-type"}, TaskList: &types.TaskList{Name: id},
				ExecutionStartToCloseTimeoutSeconds: common.Int32Ptr(100), TaskStartToCloseTimeoutSeconds: common.Int32Ptr(10)}}
	}
	chain := func(name string, start func(*types.StartWorkflowExecutionRequest), first func(id string)) {
		s.Run(name, func() {
			id := "warm-chain-" + name + "-" + uuid.New()
			run1 := s.warmStart(id, start)
			first(id)
			s.warmWaitMoved(id, run1)
			var run2 string
			for i := 0; i < 40 && (run2 == "" || run2 == run1); i++ {
				time.Sleep(500 * time.Millisecond)
				run2 = s.warmCurrentRun(id)
			}
			s.NotEqual(run1, run2, "chain must continue")
			s.warmDecide(id, "bin-1", can(id)) // second link (cron/retry/CAN) keeps progressing
			s.warmWaitMoved(id, run2)
			run3 := s.warmCurrentRun(id)
			s.NotEqual(run2, run3)
			s.warmDecide(id, "bin-1", completeD())
			s.warmWaitMoved(id, run3)
			// both earlier links are warm yet readable
			s.NotEmpty(s.warmHistory(id, run1))
			s.NotNil(s.warmDescribe(id, run2).WorkflowExecutionInfo.CloseStatus)
		})
	}
	chain("can", nil, func(id string) { s.warmDecide(id, "bin-1", can(id)) })
	chain("cron", func(r *types.StartWorkflowExecutionRequest) { r.CronSchedule = "@every 2s" },
		func(id string) { s.warmDecide(id, "bin-1", completeD()) })
	chain("retry", func(r *types.StartWorkflowExecutionRequest) {
		r.RetryPolicy = &types.RetryPolicy{InitialIntervalInSeconds: 1, BackoffCoefficient: 1, MaximumAttempts: 3}
	}, func(id string) { s.warmDecide(id, "bin-1", failD()) })
}

// 8: mode lowered to read_only after the move: warm runs stay readable, new closes stay hot.
func (s *IntegrationSuite) TestWarmStorage_ReadOnlyAfterMove_SLOW() {
	s.warmSkip()
	id := "warm-flagoff-" + uuid.New()
	run := s.warmStart(id, nil)
	s.warmDecide(id, "bin-1", completeD())
	s.warmWaitMoved(id, run)

	setWarmMode(s.T(), "", "read_only")
	defer setWarmFlag(s.T(), "", true)
	time.Sleep(8 * time.Second) // dynamic config file poll interval is 5s

	s.Equal(types.WorkflowExecutionCloseStatusCompleted, *s.warmDescribe(id, run).WorkflowExecutionInfo.CloseStatus)
	s.NotEmpty(s.warmHistory(id, run))

	id2 := "warm-flagoff2-" + uuid.New()
	run2 := s.warmStart(id2, nil)
	s.warmDecide(id2, "bin-1", completeD())
	s.warmWaitHot(id2, run2)
	// a hot closed run and a warm run must answer late writes with the same error type
	hotSig, hotTerm := s.warmLateWrites(id2, run2)
	warmSig, warmTerm := s.warmLateWrites(id, run)
	s.IsType(hotSig, warmSig)
	s.IsType(hotTerm, warmTerm)
}

// warmWaitBoth waits for a run closed in shadow mode: slim row written, executions row kept.
func (s *IntegrationSuite) warmWaitBoth(id, runID string) {
	var hot, warm int
	for i := 0; i < 60; i++ {
		if hot, warm = s.warmRows(id, runID); hot == 1 && warm == 1 {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	s.Failf("run has no slim row", "wf %s run %s: executions rows=%d closed_executions rows=%d", id, runID, hot, warm)
}

// warmEvictCache closes the run's shard so the next read goes to persistence.
func (s *IntegrationSuite) warmEvictCache(id string) {
	ctx, cancel := createContext()
	defer cancel()
	shard := int32(common.WorkflowIDToHistoryShard(id, s.TestClusterConfig.HistoryConfig.NumHistoryShards))
	s.NoError(s.HistoryClient.CloseShard(ctx, &types.CloseShardRequest{ShardID: shard}))
	time.Sleep(time.Second)
}

// Shadow: closes write the slim row but keep the executions row, reads stay on the hot row, and the slim row
// matches what the hot row projects to.
func (s *IntegrationSuite) TestWarmStorage_Shadow_SLOW() {
	s.warmSkip()
	setWarmMode(s.T(), "", "shadow")
	defer func() {
		setWarmFlag(s.T(), "", true)
		warmFlagOn = true
		time.Sleep(7 * time.Second)
	}()
	warmFlagOn = false
	time.Sleep(8 * time.Second) // dynamic config file poll interval is 5s

	var runs [][2]string
	for i := 0; i < 3; i++ {
		id := "warm-shadow-" + uuid.New()
		runs = append(runs, [2]string{id, s.warmSetupDone(id, "bin-1", completeD())})
	}
	domainID := s.warmDomainID()
	scope := tally.NewTestScope("", nil)
	mgr := s.warmExecMgrMode("shadow", metrics.NewClient(scope, metrics.History, metrics.MigrationConfig{}))
	defer mgr.Close()
	ctx, cancel := createContext()
	defer cancel()
	for _, r := range runs {
		s.warmWaitBoth(r[0], r[1])
		s.warmEvictCache(r[0])
		s.Equal("COMPLETED", s.warmStatus(r[0], r[1]))
		s.NotEmpty(s.warmHistory(r[0], r[1]))
		hot, warm := s.warmRows(r[0], r[1])
		s.Equal([]int{1, 1}, []int{hot, warm}, "shadow keeps both rows")

		// a fresh read through a shadow manager compares the slim row with the hot row
		shard := common.IntPtr(common.WorkflowIDToHistoryShard(r[0], s.TestClusterConfig.HistoryConfig.NumHistoryShards))
		resp, err := mgr.GetWorkflowExecution(ctx, &persistence.GetWorkflowExecutionRequest{
			ShardID: shard, DomainID: domainID, DomainName: s.DomainName,
			Execution: types.WorkflowExecution{WorkflowID: r[0], RunID: r[1]}})
		s.NoError(err)
		s.Equal(persistence.WorkflowStateCompleted, resp.State.ExecutionInfo.State)
	}
	var mismatches int64
	for _, c := range scope.Snapshot().Counters() {
		if c.Name() == "warm_storage_shadow_mismatches" {
			mismatches += c.Value()
		}
	}
	s.Zero(mismatches)
}

// on, then read_only, then off: a moved run stays readable in read_only and is not found in off.
func (s *IntegrationSuite) TestWarmStorage_OnReadOnlyOff_SLOW() {
	s.warmSkip()
	id := "warm-onroff-" + uuid.New()
	run := s.warmStart(id, nil)
	s.warmDecide(id, "bin-1", completeD())
	s.warmWaitMoved(id, run)
	defer func() {
		setWarmFlag(s.T(), "", true)
		time.Sleep(7 * time.Second)
	}()

	setWarmMode(s.T(), "", "read_only")
	time.Sleep(8 * time.Second)
	s.warmEvictCache(id)
	s.Equal("COMPLETED", s.warmStatus(id, run))
	s.NotEmpty(s.warmHistory(id, run))

	setWarmMode(s.T(), "", "off")
	time.Sleep(8 * time.Second)
	s.warmEvictCache(id)
	ctx, cancel := createContext()
	defer cancel()
	_, err := s.Engine.DescribeWorkflowExecution(ctx, &types.DescribeWorkflowExecutionRequest{
		Domain: s.DomainName, Execution: &types.WorkflowExecution{WorkflowID: id, RunID: run}})
	s.IsType(&types.EntityNotExistsError{}, err)
	hot, warm := s.warmRows(id, run)
	s.Equal([]int{0, 1}, []int{hot, warm}, "the slim row is left behind, which is why off comes after read_only")
}
