package host

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/pborman/uuid"
	"go.uber.org/cadence/activity"
	"go.uber.org/cadence/testsuite"
	"go.uber.org/mock/gomock"

	"github.com/uber/cadence/common"
	"github.com/uber/cadence/common/backoff"
	"github.com/uber/cadence/common/cache"
	"github.com/uber/cadence/common/constants"
	"github.com/uber/cadence/common/dynamicconfig/dynamicproperties"
	"github.com/uber/cadence/common/log"
	"github.com/uber/cadence/common/metrics"
	"github.com/uber/cadence/common/persistence"
	"github.com/uber/cadence/common/reconciliation/entity"
	"github.com/uber/cadence/common/reconciliation/invariant"
	"github.com/uber/cadence/common/types"
	"github.com/uber/cadence/service/worker/scanner/history"
)

// Feature tests for warm storage. Each one runs the same scenario twice: once with the flag off
// (the closed run stays hot) and once with it on (the closed run is moved, which the test verifies
// through the raw tables before acting), then requires identical observable outcomes.

// warmFlagOn mirrors the flag state the dynamic config file currently holds.
var warmFlagOn = true

func (s *IntegrationSuite) warmFlag(on bool) {
	if warmFlagOn == on {
		return
	}
	setWarmFlag(s.T(), "", on)
	warmFlagOn = on
	time.Sleep(7 * time.Second) // dynamic config file poll interval is 5s
}

// warmFlagDomains writes a per-domain flag: def everywhere, overridden by on[domainID].
func (s *IntegrationSuite) warmFlagDomains(def bool, on map[string]bool) {
	var b strings.Builder
	b.WriteString(warmBaseDC + "\nsystem.enableWarmStorage:\n")
	for id, v := range on {
		fmt.Fprintf(&b, "- value: %v\n  constraints:\n    domainID: %q\n", v, id)
	}
	fmt.Fprintf(&b, "- value: %v\n  constraints: {}\n", def)
	s.NoError(os.WriteFile(warmDCPath, []byte(b.String()), 0o644))
	warmFlagOn = def
	time.Sleep(12 * time.Second)
}

func (s *IntegrationSuite) warmBothWays(f func(warm bool) []string) {
	s.warmSkip()
	s.warmFlag(false)
	hot := f(false)
	s.warmFlag(true)
	warm := f(true)
	s.NotEmpty(hot)
	s.T().Log("OUTCOME hot:\n" + strings.Join(hot, "\n") + "\nOUTCOME warm:\n" + strings.Join(warm, "\n"))
	s.Equal(hot, warm, "outcome on a warm run must equal the outcome on a hot closed run")
}

// warmSettle waits until the run is where the mode says (moved or hot), then evicts the history cache.
func (s *IntegrationSuite) warmSettle(warm bool, ids ...[2]string) {
	for _, p := range ids {
		if warm {
			s.warmWaitMoved(p[0], p[1])
		}
	}
	if !warm {
		time.Sleep(2 * time.Second)
		for _, p := range ids {
			hot, w := s.warmRows(p[0], p[1])
			s.Equal([]int{1, 0}, []int{hot, w}, "run must stay hot")
		}
	}
	ctx, cancel := createContext()
	defer cancel()
	for _, p := range ids {
		shard := int32(common.WorkflowIDToHistoryShard(p[0], s.TestClusterConfig.HistoryConfig.NumHistoryShards))
		s.NoError(s.HistoryClient.CloseShard(ctx, &types.CloseShardRequest{ShardID: shard}))
	}
	time.Sleep(time.Second)
}

func (s *IntegrationSuite) warmIn(domain string, f func()) {
	old := s.DomainName
	s.DomainName = domain
	defer func() { s.DomainName = old }()
	f()
}

func errName(err error) string {
	if err == nil {
		return "nil"
	}
	return fmt.Sprintf("%T", err)
}

func (s *IntegrationSuite) warmSignal(id, run, name string) error {
	ctx, cancel := createContext()
	defer cancel()
	return s.Engine.SignalWorkflowExecution(ctx, &types.SignalWorkflowExecutionRequest{
		Domain: s.DomainName, WorkflowExecution: &types.WorkflowExecution{WorkflowID: id, RunID: run}, SignalName: name, Identity: "warm"})
}

// warmSetupDone starts a run, completes two decisions and closes it with the given decision.
func (s *IntegrationSuite) warmSetupDone(id, checksum string, closer *types.Decision) string {
	run := s.warmStart(id, nil)
	s.warmDecide(id, checksum)
	s.NoError(s.warmSignal(id, run, "go"))
	s.warmDecide(id, checksum, closer)
	return run
}

func (s *IntegrationSuite) warmTypes(id, run string) string {
	var out []string
	for _, e := range s.warmHistory(id, run) {
		out = append(out, e.EventType.String())
	}
	return strings.Join(out, ",")
}

func (s *IntegrationSuite) warmStatus(id, run string) string {
	d := s.warmDescribe(id, run).WorkflowExecutionInfo
	if d.CloseStatus == nil {
		return "open"
	}
	return d.CloseStatus.String()
}

func (s *IntegrationSuite) warmWaitCurrentNot(id string, not ...string) string {
	var cur string
	for i := 0; i < 60; i++ {
		cur = s.warmCurrentRun(id)
		found := false
		for _, n := range not {
			found = found || n == cur
		}
		if !found {
			return cur
		}
		time.Sleep(500 * time.Millisecond)
	}
	s.Failf("current run did not change", "wf %s still %s", id, cur)
	return cur
}

func (s *IntegrationSuite) warmWaitStatus(id, run, want string) string {
	var got string
	for i := 0; i < 40; i++ {
		if got = s.warmStatus(id, run); got == want {
			return got
		}
		time.Sleep(500 * time.Millisecond)
	}
	return got
}

func (s *IntegrationSuite) warmDecisionIDs(id, run string, t types.EventType, first bool) int64 {
	var res int64
	for _, e := range s.warmHistory(id, run) {
		if *e.EventType == t {
			res = e.ID
			if first {
				return res
			}
		}
	}
	return res
}

func (s *IntegrationSuite) warmReset(id, run string, eventID int64) (string, error) {
	ctx, cancel := createContext()
	defer cancel()
	resp, err := s.Engine.ResetWorkflowExecution(ctx, &types.ResetWorkflowExecutionRequest{
		Domain: s.DomainName, WorkflowExecution: &types.WorkflowExecution{WorkflowID: id, RunID: run},
		Reason: "warm", DecisionFinishEventID: eventID, RequestID: uuid.New()})
	if err != nil {
		return "", err
	}
	return resp.RunID, nil
}

// warmResetOutcome resets, closes the new run and reports what happened.
func (s *IntegrationSuite) warmResetOutcome(id, run string, eventID int64) []string {
	newRun, err := s.warmReset(id, run, eventID)
	out := []string{"reset=" + errName(err)}
	if err != nil {
		return out
	}
	s.NotEqual(run, newRun)
	s.warmDecide(id, "bin-2", completeD())
	return append(out, "new="+s.warmTypes(id, newRun), "newstatus="+s.warmStatus(id, newRun), "base="+s.warmStatus(id, run), "current="+fmt.Sprint(s.warmCurrentRun(id) == newRun))
}

// Reset: by event ID, to the last decision completed, and with an invalid event ID.
func (s *IntegrationSuite) TestWarmStorage_Reset_SLOW() {
	s.warmBothWays(func(warm bool) []string {
		ids := map[string][2]string{}
		for _, n := range []string{"eventid", "lastdecision", "invalid"} {
			id := "warm-reset-" + n + "-" + uuid.New()
			ids[n] = [2]string{id, s.warmSetupDone(id, "bin-1", completeD())}
		}
		s.warmSettle(warm, ids["eventid"], ids["lastdecision"], ids["invalid"])
		var out []string
		for _, n := range []string{"eventid", "lastdecision", "invalid"} {
			id, run := ids[n][0], ids[n][1]
			var eid int64
			switch n {
			case "eventid":
				eid = s.warmDecisionIDs(id, run, types.EventTypeDecisionTaskCompleted, true)
			case "lastdecision":
				eid = s.warmDecisionIDs(id, run, types.EventTypeDecisionTaskCompleted, false)
			default:
				eid = 1
			}
			for _, l := range s.warmResetOutcome(id, run, eid) {
				out = append(out, n+"/"+l)
			}
		}
		return out
	})
}

// Batch reset in the style of `cadence workflow reset-batch`: Describe the current run, pick the reset
// point by bad binary checksum (the rebuilt AutoResetPoints) or by the last decision, then reset.
func (s *IntegrationSuite) TestWarmStorage_BatchReset_SLOW() {
	s.warmBothWays(func(warm bool) []string {
		bad := "bad-" + uuid.New()
		var ids [][2]string
		for i := 0; i < 3; i++ {
			id := fmt.Sprintf("warm-batch-%d-%s", i, uuid.New())
			ids = append(ids, [2]string{id, s.warmSetupDone(id, bad, completeD())})
		}
		s.warmSettle(warm, ids...)
		var out []string
		for i, p := range ids {
			id := p[0]
			desc := s.warmDescribe(id, "").WorkflowExecutionInfo // empty run ID: current run, as the CLI does
			var sums []string
			var point *types.ResetPointInfo
			if desc.AutoResetPoints != nil {
				for _, pt := range desc.AutoResetPoints.Points {
					sums = append(sums, fmt.Sprintf("%v:%v:%v", pt.BinaryChecksum == bad, pt.FirstDecisionCompletedID, pt.Resettable))
					if pt.BinaryChecksum == bad && point == nil {
						point = pt
					}
				}
			}
			out = append(out, fmt.Sprintf("%d/points=%v status=%v", i, sums, *desc.CloseStatus))
			if point == nil {
				out = append(out, fmt.Sprintf("%d/no bad binary point", i))
				continue
			}
			s.Equal(p[1], point.RunID)
			for _, l := range s.warmResetOutcome(id, point.RunID, point.FirstDecisionCompletedID) {
				out = append(out, fmt.Sprintf("%d/%s", i, l))
			}
		}
		return out
	})
}

func (s *IntegrationSuite) warmBadBinaries(domain string, bins ...string) {
	m := map[string]*types.BadBinaryInfo{}
	for _, b := range bins {
		m[b] = &types.BadBinaryInfo{Reason: "warm test", Operator: "warm"}
	}
	var err error
	for i := 0; i < 20; i++ { // the frontend rate limits domain updates
		ctx, cancel := createContext()
		_, err = s.Engine.UpdateDomain(ctx, &types.UpdateDomainRequest{Name: domain, BadBinaries: &types.BadBinaries{Binaries: m}})
		cancel()
		if err == nil {
			break
		}
		time.Sleep(3 * time.Second)
	}
	s.NoError(err)
	for i := 0; i < 40; i++ { // wait until the frontend's domain cache serves the update
		ctx, cancel := createContext()
		dd, err := s.Engine.DescribeDomain(ctx, &types.DescribeDomainRequest{Name: &domain})
		cancel()
		s.NoError(err)
		if len(dd.Configuration.BadBinaries.Binaries) == len(bins) {
			break
		}
		time.Sleep(time.Second)
	}
	time.Sleep(cache.DomainCacheRefreshInterval + time.Second) // and the history service's
}

// Bad binary auto-reset whose base run is Warm: run 1 decides under a binary and continues as new, run 1 is
// moved, then a signal on run 2 schedules the auto-reset of run 1's reset point. The binary is bad from the
// domain's creation (a domain update reaches the running cluster only after the cache refresh, which is slow on
// the real clock), but the CAN decision closes run 1 so no reset is scheduled while it runs.
func (s *IntegrationSuite) TestWarmStorage_AutoResetFromWarmBase_SLOW() {
	s.warmSkip()
	// OPEN: in this onebox a domain update (BadBinaries) is never served by DescribeDomain or the history
	// service's domain cache, even 40s later, with the flag off as well. So the auto-reset never fires and the
	// scenario cannot be exercised yet. Reset by bad binary checksum on a warm run is covered by BatchReset.
	s.T().Skip("cannot apply a bad binary to a running onebox domain")
	domain := s.RandomizeStr("warm-badbin-domain")
	s.warmRegisterDomain(domain, 1)
	var bad []string
	s.warmBothWays(func(warm bool) (out []string) {
		s.warmIn(domain, func() {
			id := "warm-autoreset-" + uuid.New()
			b := "bad-" + uuid.New()
			bad = append(bad, b)
			run1 := s.warmStart(id, nil)
			s.warmDecide(id, b, &types.Decision{
				DecisionType: types.DecisionTypeContinueAsNewWorkflowExecution.Ptr(),
				ContinueAsNewWorkflowExecutionDecisionAttributes: &types.ContinueAsNewWorkflowExecutionDecisionAttributes{
					WorkflowType: &types.WorkflowType{Name: "warm-type"}, TaskList: &types.TaskList{Name: id},
					ExecutionStartToCloseTimeoutSeconds: common.Int32Ptr(100), TaskStartToCloseTimeoutSeconds: common.Int32Ptr(10)}})
			run2 := s.warmWaitCurrentNot(id, run1)
			s.warmSettle(warm, [2]string{id, run1})
			before := s.warmDescribe(id, run2).WorkflowExecutionInfo
			n := 0
			if before.AutoResetPoints != nil {
				n = len(before.AutoResetPoints.Points)
			}
			out = append(out, fmt.Sprintf("run2 inherited points=%d", n))
			s.warmBadBinaries(domain, bad...)
			s.NoError(s.warmSignal(id, "", "trigger"))
			run3 := s.warmWaitCurrentNot(id, run1, run2)
			out = append(out, "run1="+s.warmStatus(id, run1), "run2="+s.warmStatus(id, run2), "run3="+s.warmTypes(id, run3))
			s.warmDecide(id, "good", completeD())
			out = append(out, "run3="+s.warmStatus(id, run3))
		})
		return out
	})
}

func (s *IntegrationSuite) TestWarmStorage_Restart_SLOW() {
	s.warmBothWays(func(warm bool) []string {
		var ids [][2]string
		names := []string{"completed", "failed"}
		for _, n := range names {
			id := "warm-restart-" + n + "-" + uuid.New()
			c := completeD()
			if n == "failed" {
				c = failD()
			}
			ids = append(ids, [2]string{id, s.warmSetupDone(id, "bin-1", c)})
		}
		s.warmSettle(warm, ids...)
		var out []string
		for i, p := range ids {
			ctx, cancel := createContext()
			resp, err := s.Engine.RestartWorkflowExecution(ctx, &types.RestartWorkflowExecutionRequest{
				Domain: s.DomainName, WorkflowExecution: &types.WorkflowExecution{WorkflowID: p[0], RunID: p[1]}, Identity: "warm"})
			cancel()
			out = append(out, names[i]+"/restart="+errName(err))
			if err != nil {
				continue
			}
			s.NotEqual(p[1], resp.RunID)
			out = append(out, names[i]+"/new="+s.warmTypes(p[0], resp.RunID))
			s.warmDecide(p[0], "bin-2", completeD())
			out = append(out, names[i]+"/newstatus="+s.warmStatus(p[0], resp.RunID), names[i]+"/base="+s.warmStatus(p[0], p[1]))
		}
		return out
	})
}

// Signal-with-start and every ID reuse policy against a closed run (completed and failed).
func (s *IntegrationSuite) TestWarmStorage_StartReusePolicies_SLOW() {
	policies := []types.WorkflowIDReusePolicy{
		types.WorkflowIDReusePolicyAllowDuplicate, types.WorkflowIDReusePolicyAllowDuplicateFailedOnly,
		types.WorkflowIDReusePolicyRejectDuplicate, types.WorkflowIDReusePolicyTerminateIfRunning}
	s.warmBothWays(func(warm bool) []string {
		type c struct {
			name, id, run string
			policy        int // -1: signal-with-start
			failed        bool
		}
		var cases []c
		var ids [][2]string
		for _, failed := range []bool{false, true} {
			for p := -1; p < len(policies); p++ {
				name := "sws"
				if p >= 0 {
					name = policies[p].String()
				}
				name = fmt.Sprintf("%s/failed=%v", name, failed)
				id := "warm-reuse-" + uuid.New()
				closer := completeD()
				if failed {
					closer = failD()
				}
				run := s.warmSetupDone(id, "bin-1", closer)
				cases = append(cases, c{name, id, run, p, failed})
				ids = append(ids, [2]string{id, run})
			}
		}
		s.warmSettle(warm, ids...)
		var out []string
		for _, tc := range cases {
			ctx, cancel := createContext()
			var newRun string
			var err error
			if tc.policy < 0 {
				var r *types.StartWorkflowExecutionResponse
				r, err = s.Engine.SignalWithStartWorkflowExecution(ctx, &types.SignalWithStartWorkflowExecutionRequest{
					Domain: s.DomainName, WorkflowID: tc.id, WorkflowType: &types.WorkflowType{Name: "warm-type"},
					TaskList:                            &types.TaskList{Name: tc.id},
					ExecutionStartToCloseTimeoutSeconds: common.Int32Ptr(100), TaskStartToCloseTimeoutSeconds: common.Int32Ptr(10),
					Identity: "warm", RequestID: uuid.New(), SignalName: "sws"})
				if err == nil {
					newRun = r.RunID
				}
			} else {
				newRun, err = s.warmTryStart(tc.id, policies[tc.policy])
			}
			cancel()
			out = append(out, tc.name+"/start="+errName(err))
			if err == nil {
				s.NotEqual(tc.run, newRun)
				out = append(out, tc.name+"/new="+s.warmTypes(tc.id, newRun), tc.name+"/old="+s.warmStatus(tc.id, tc.run))
			}
		}
		return out
	})
}

func (s *IntegrationSuite) warmTryStart(id string, p types.WorkflowIDReusePolicy) (string, error) {
	ctx, cancel := createContext()
	defer cancel()
	r, err := s.Engine.StartWorkflowExecution(ctx, &types.StartWorkflowExecutionRequest{
		RequestID: uuid.New(), Domain: s.DomainName, WorkflowID: id, WorkflowType: &types.WorkflowType{Name: "warm-type"},
		TaskList:                            &types.TaskList{Name: id},
		ExecutionStartToCloseTimeoutSeconds: common.Int32Ptr(100), TaskStartToCloseTimeoutSeconds: common.Int32Ptr(10),
		WorkflowIDReusePolicy: &p, Identity: "warm"})
	if err != nil {
		return "", err
	}
	return r.RunID, nil
}

// Query on a closed run: the worker replays the history it gets for the warm run.
func (s *IntegrationSuite) TestWarmStorage_Query_SLOW() {
	s.warmBothWays(func(warm bool) []string {
		id := "warm-query-" + uuid.New()
		run := s.warmSetupDone(id, "bin-1", completeD())
		s.warmSettle(warm, [2]string{id, run})
		type res struct {
			resp *types.QueryWorkflowResponse
			err  error
		}
		ch := make(chan res, 1)
		go func() {
			ctx, cancel := createContext()
			defer cancel()
			r, err := s.Engine.QueryWorkflow(ctx, &types.QueryWorkflowRequest{
				Domain: s.DomainName, Execution: &types.WorkflowExecution{WorkflowID: id, RunID: run},
				Query: &types.WorkflowQuery{QueryType: "q", QueryArgs: []byte("a")}})
			ch <- res{r, err}
		}()
		ctx, cancel := createContext()
		defer cancel()
		poll, err := s.Engine.PollForDecisionTask(ctx, &types.PollForDecisionTaskRequest{Domain: s.DomainName, TaskList: &types.TaskList{Name: id}, Identity: "warm"})
		s.NoError(err)
		s.NotNil(poll.Query, "expected a query task")
		out := []string{"query=" + poll.Query.QueryType + "/" + string(poll.Query.QueryArgs), fmt.Sprintf("events=%d", len(poll.History.Events))}
		s.NoError(s.Engine.RespondQueryTaskCompleted(ctx, &types.RespondQueryTaskCompletedRequest{
			TaskToken: poll.TaskToken, CompletedType: types.QueryTaskCompletedTypeCompleted.Ptr(), QueryResult: []byte("answer")}))
		r := <-ch
		s.NoError(r.err)
		return append(out, "result="+string(r.resp.QueryResult))
	})
}

func (s *IntegrationSuite) warmDescribeLines(id, run string) []string {
	d := s.warmDescribe(id, run)
	i := d.WorkflowExecutionInfo
	out := []string{
		"status=" + s.warmStatus(id, run), fmt.Sprintf("len=%d", i.HistoryLength), "type=" + i.Type.Name,
		"memo=" + string(i.Memo.Fields["k"]), fmt.Sprintf("activities=%d children=%d decision=%v", len(d.PendingActivities), len(d.PendingChildren), d.PendingDecision),
		fmt.Sprintf("parent=%v cron=%v", i.ParentExecution != nil, i.IsCron),
		fmt.Sprintf("tl=%v start=%d/%d", d.ExecutionConfiguration.TaskList.Name == id, *d.ExecutionConfiguration.ExecutionStartToCloseTimeoutSeconds, *d.ExecutionConfiguration.TaskStartToCloseTimeoutSeconds),
	}
	if i.AutoResetPoints != nil {
		for _, p := range i.AutoResetPoints.Points {
			out = append(out, fmt.Sprintf("point=%s:%d:%v:%v", p.BinaryChecksum, p.FirstDecisionCompletedID, p.Resettable, p.RunID == run))
		}
	}
	return out
}

// Describe, then DescribeMutableState through the history client and the admin handler.
func (s *IntegrationSuite) TestWarmStorage_DescribeAndAdmin_SLOW() {
	s.warmBothWays(func(warm bool) []string {
		id := "warm-describe-" + uuid.New()
		run := s.warmSetupDone(id, "bin-1", completeD())
		s.warmSettle(warm, [2]string{id, run})
		out := s.warmDescribeLines(id, run)
		out = append(out, s.warmDescribeLines(id, "")...) // current run

		ctx, cancel := createContext()
		defer cancel()
		admin, err := s.AdminClient.DescribeWorkflowExecution(ctx, &types.AdminDescribeWorkflowExecutionRequest{
			Domain: s.DomainName, Execution: &types.WorkflowExecution{WorkflowID: id, RunID: run}})
		s.NoError(err)
		hist, err := s.HistoryClient.DescribeMutableState(ctx, &types.DescribeMutableStateRequest{
			DomainUUID: s.warmDomainID(), Execution: &types.WorkflowExecution{WorkflowID: id, RunID: run}})
		s.NoError(err)
		for name, raw := range map[string]string{"admin": admin.MutableStateInDatabase, "history": hist.MutableStateInDatabase} {
			var ms map[string]json.RawMessage
			s.NoError(json.Unmarshal([]byte(raw), &ms), name)
			var keys []string
			for k, v := range ms {
				if string(v) != "null" && string(v) != "{}" && string(v) != "[]" {
					keys = append(keys, k)
				}
			}
			sort.Strings(keys)
			out = append(out, name+" keys="+strings.Join(keys, ","))
			var info map[string]any
			s.NoError(json.Unmarshal(ms["ExecutionInfo"], &info))
			for _, k := range []string{"State", "CloseStatus", "NextEventID", "WorkflowTypeName", "TaskList", "LastFirstEventID"} {
				v := info[k]
				if k == "TaskList" {
					v = v == id
				}
				out = append(out, fmt.Sprintf("%s %s=%v", name, k, v))
			}
		}
		return out
	})
}

// KNOWN FAIL until workstream D item 6 (writes to moved runs): today the refresh of a warm run ends in a
// conditional-update failure (InternalServiceError) because the executions row is gone. Do not weaken.
// RefreshWorkflowTasks on a closed run. A hot closed run regenerates its tasks (and with the flag on moves
// again); a warm run has no row to update. Either way the run must stay readable.
func (s *IntegrationSuite) TestWarmStorage_RefreshTasks_SLOW() {
	s.warmBothWays(func(warm bool) []string {
		id := "warm-refresh-" + uuid.New()
		run := s.warmSetupDone(id, "bin-1", completeD())
		s.warmSettle(warm, [2]string{id, run})
		ctx, cancel := createContext()
		defer cancel()
		err := s.AdminClient.RefreshWorkflowTasks(ctx, &types.RefreshWorkflowTasksRequest{
			Domain: s.DomainName, Execution: &types.WorkflowExecution{WorkflowID: id, RunID: run}})
		out := []string{"refresh=" + errName(err)}
		time.Sleep(3 * time.Second)
		if warm {
			s.warmWaitMoved(id, run) // a warm run must still be (or again be) moved after the refresh
		}
		return append(out, s.warmDescribeLines(id, run)...)
	})
}

func (s *IntegrationSuite) warmChildDecision(child string, policy types.ParentClosePolicy) *types.Decision {
	return &types.Decision{
		DecisionType: types.DecisionTypeStartChildWorkflowExecution.Ptr(),
		StartChildWorkflowExecutionDecisionAttributes: &types.StartChildWorkflowExecutionDecisionAttributes{
			Domain: s.DomainName, WorkflowID: child, WorkflowType: &types.WorkflowType{Name: "warm-child"}, TaskList: &types.TaskList{Name: child},
			ExecutionStartToCloseTimeoutSeconds: common.Int32Ptr(100), TaskStartToCloseTimeoutSeconds: common.Int32Ptr(10),
			ParentClosePolicy: &policy}}
}

// Parent closes while the child runs, under each parent close policy; then the child finishes. The parent
// stays hot with the flag on because its child is still pending when it closes.
func (s *IntegrationSuite) TestWarmStorage_ParentClosePolicies_SLOW() {
	s.warmBothWays(func(warm bool) []string {
		policies := []types.ParentClosePolicy{types.ParentClosePolicyTerminate, types.ParentClosePolicyRequestCancel, types.ParentClosePolicyAbandon}
		type c struct{ parent, child, run string }
		var cs []c
		var ids [][2]string
		for _, p := range policies {
			parent, child := "warm-parent-"+uuid.New(), "warm-child-"+uuid.New()
			run := s.warmStart(parent, nil)
			s.warmDecide(parent, "bin-1", s.warmChildDecision(child, p))
			s.warmDecide(parent, "bin-1", completeD()) // decision scheduled by the child's start event
			cs = append(cs, c{parent, child, run})
			ids = append(ids, [2]string{parent, run})
		}
		// A parent that closes while its child still runs lists the child in Describe, which the slim row cannot
		// carry, so it stays hot in both modes (6f3562b87). Wait for the close, check that, and evict.
		s.warmSettle(false, ids...)
		var out []string
		for i, p := range policies {
			n := p.String()
			parent, child := cs[i].parent, cs[i].child
			childRun := s.warmCurrentRun(child)
			switch p {
			case types.ParentClosePolicyTerminate:
				out = append(out, n+"/child="+s.warmWaitStatus(child, childRun, "TERMINATED"))
			case types.ParentClosePolicyRequestCancel:
				ctx, cancel := createContext()
				poll, err := s.Engine.PollForDecisionTask(ctx, &types.PollForDecisionTaskRequest{Domain: s.DomainName, TaskList: &types.TaskList{Name: child}, Identity: "warm"})
				cancel()
				s.NoError(err)
				var types_ []string
				for _, e := range poll.History.Events {
					types_ = append(types_, e.EventType.String())
				}
				out = append(out, n+"/childhistory="+strings.Join(types_, ","))
				ctx, cancel = createContext()
				_, err = s.Engine.RespondDecisionTaskCompleted(ctx, &types.RespondDecisionTaskCompletedRequest{TaskToken: poll.TaskToken, Identity: "warm", Decisions: []*types.Decision{{
					DecisionType: types.DecisionTypeCancelWorkflowExecution.Ptr(),
					CancelWorkflowExecutionDecisionAttributes: &types.CancelWorkflowExecutionDecisionAttributes{}}}})
				cancel()
				s.NoError(err)
				out = append(out, n+"/child="+s.warmWaitStatus(child, childRun, "CANCELED"))
			default:
				out = append(out, n+"/child="+s.warmStatus(child, childRun))
				s.warmDecide(child, "bin-1", completeD())
				out = append(out, n+"/childafter="+s.warmWaitStatus(child, childRun, "COMPLETED"))
			}
			time.Sleep(2 * time.Second) // let the child's completion reach the (closed) parent
			out = append(out, n+"/parent="+s.warmTypes(parent, cs[i].run), n+"/parentstatus="+s.warmStatus(parent, cs[i].run))
		}
		return out
	})
}

// The child completes and moves before the parent records it; the parent then still sees the child's completion.
func (s *IntegrationSuite) TestWarmStorage_ChildMovesBeforeParent_SLOW() {
	s.warmBothWays(func(warm bool) []string {
		parent, child := "warm-parent2-"+uuid.New(), "warm-child2-"+uuid.New()
		prun := s.warmStart(parent, nil)
		s.warmDecide(parent, "bin-1", s.warmChildDecision(child, types.ParentClosePolicyAbandon))
		var crun string
		for i := 0; i < 40 && crun == ""; i++ {
			time.Sleep(500 * time.Millisecond)
			ctx, cancel := createContext()
			r, err := s.Engine.DescribeWorkflowExecution(ctx, &types.DescribeWorkflowExecutionRequest{Domain: s.DomainName, Execution: &types.WorkflowExecution{WorkflowID: child}})
			cancel()
			if err == nil {
				crun = r.WorkflowExecutionInfo.Execution.RunID
			}
		}
		s.NotEmpty(crun)
		s.warmDecide(child, "bin-1", completeD())
		s.warmSettle(warm, [2]string{child, crun})
		// the parent's decision task carrying the child events
		s.warmDecide(parent, "bin-1", completeD())
		return []string{
			"parent=" + s.warmTypes(parent, prun), "parentstatus=" + s.warmStatus(parent, prun),
			"child=" + s.warmStatus(child, crun), fmt.Sprint(s.warmDescribe(child, crun).WorkflowExecutionInfo.ParentExecution != nil),
		}
	})
}

// A running workflow cancels and signals a closed (warm) workflow of the same domain.
func (s *IntegrationSuite) TestWarmStorage_ExternalCancelAndSignal_SLOW() {
	s.warmBothWays(func(warm bool) []string {
		tid := "warm-target-" + uuid.New()
		trun := s.warmSetupDone(tid, "bin-1", completeD())
		s.warmSettle(warm, [2]string{tid, trun})
		var out []string
		for _, kind := range []string{"cancel", "signal"} {
			wid := "warm-source-" + kind + "-" + uuid.New()
			wrun := s.warmStart(wid, nil)
			d := &types.Decision{}
			if kind == "cancel" {
				d.DecisionType = types.DecisionTypeRequestCancelExternalWorkflowExecution.Ptr()
				d.RequestCancelExternalWorkflowExecutionDecisionAttributes = &types.RequestCancelExternalWorkflowExecutionDecisionAttributes{
					Domain: s.DomainName, WorkflowID: tid, RunID: trun}
			} else {
				d.DecisionType = types.DecisionTypeSignalExternalWorkflowExecution.Ptr()
				d.SignalExternalWorkflowExecutionDecisionAttributes = &types.SignalExternalWorkflowExecutionDecisionAttributes{
					Domain: s.DomainName, Execution: &types.WorkflowExecution{WorkflowID: tid, RunID: trun}, SignalName: "ext"}
			}
			s.warmDecide(wid, "bin-1", d)
			time.Sleep(3 * time.Second)
			var cause []string
			for _, e := range s.warmHistory(wid, wrun) {
				cause = append(cause, e.EventType.String())
				if e.RequestCancelExternalWorkflowExecutionFailedEventAttributes != nil {
					cause = append(cause, e.RequestCancelExternalWorkflowExecutionFailedEventAttributes.Cause.String())
				}
				if e.SignalExternalWorkflowExecutionFailedEventAttributes != nil {
					cause = append(cause, e.SignalExternalWorkflowExecutionFailedEventAttributes.Cause.String())
				}
			}
			out = append(out, kind+"/"+strings.Join(cause, ","))
		}
		return append(out, "target="+s.warmTypes(tid, trun), "targetstatus="+s.warmStatus(tid, trun))
	})
}

// List closed / open / count / scan after the move.
func (s *IntegrationSuite) TestWarmStorage_Visibility_SLOW() {
	s.warmBothWays(func(warm bool) []string {
		id := "warm-vis-" + uuid.New()
		run := s.warmSetupDone(id, "bin-1", failD())
		s.warmSettle(warm, [2]string{id, run})
		var out []string
		list := func() (statuses []string) {
			for i := 0; i < 30; i++ {
				ctx, cancel := createContext()
				r, err := s.Engine.ListClosedWorkflowExecutions(ctx, &types.ListClosedWorkflowExecutionsRequest{
					Domain: s.DomainName, MaximumPageSize: 10, ExecutionFilter: &types.WorkflowExecutionFilter{WorkflowID: id},
					StartTimeFilter: &types.StartTimeFilter{EarliestTime: common.Int64Ptr(0), LatestTime: common.Int64Ptr(time.Now().Add(time.Hour).UnixNano())}})
				cancel()
				s.NoError(err)
				statuses = nil
				for _, e := range r.Executions {
					statuses = append(statuses, fmt.Sprintf("%s:%v:%v", e.CloseStatus, e.Execution.RunID == run, e.CloseTime != nil))
				}
				if len(statuses) > 0 {
					return
				}
				time.Sleep(time.Second)
			}
			return
		}
		out = append(out, fmt.Sprint("closed=", list()))
		ctx, cancel := createContext()
		defer cancel()
		open, err := s.Engine.ListOpenWorkflowExecutions(ctx, &types.ListOpenWorkflowExecutionsRequest{
			Domain: s.DomainName, MaximumPageSize: 10, ExecutionFilter: &types.WorkflowExecutionFilter{WorkflowID: id},
			StartTimeFilter: &types.StartTimeFilter{EarliestTime: common.Int64Ptr(0), LatestTime: common.Int64Ptr(time.Now().Add(time.Hour).UnixNano())}})
		s.NoError(err)
		out = append(out, fmt.Sprint("open=", len(open.Executions)))
		q := fmt.Sprintf("WorkflowID = '%s'", id)
		cnt, err := s.Engine.CountWorkflowExecutions(ctx, &types.CountWorkflowExecutionsRequest{Domain: s.DomainName, Query: q})
		out = append(out, "count="+errName(err))
		if err == nil {
			out = append(out, fmt.Sprint("count=", cnt.Count))
		}
		_, err = s.Engine.ScanWorkflowExecutions(ctx, &types.ListWorkflowExecutionsRequest{Domain: s.DomainName, PageSize: 10, Query: q})
		out = append(out, "scan="+errName(err))
		return out
	})
}

func (s *IntegrationSuite) warmRegisterDomain(name string, retentionDays int32, badBinaries ...string) {
	bad := map[string]*types.BadBinaryInfo{}
	for _, b := range badBinaries {
		bad[b] = &types.BadBinaryInfo{Reason: "warm test", Operator: "warm"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), defaultTestPersistenceTimeout)
	defer cancel()
	cluster := s.TestCluster.testBase.ClusterMetadata.GetCurrentClusterName()
	_, err := s.TestCluster.testBase.DomainManager.CreateDomain(ctx, &persistence.CreateDomainRequest{
		Info:   &persistence.DomainInfo{ID: uuid.New(), Name: name, Status: persistence.DomainStatusRegistered},
		Config: &persistence.DomainConfig{Retention: retentionDays, BadBinaries: types.BadBinaries{Binaries: bad}},
		ReplicationConfig: &persistence.DomainReplicationConfig{
			ActiveClusterName: cluster, Clusters: []*persistence.ClusterReplicationConfig{{ClusterName: cluster}}},
		FailoverVersion: constants.EmptyVersion,
	})
	s.NoError(err)
}

func (s *IntegrationSuite) warmHistoryDeleted(id, run string) bool {
	for i := 0; i < 40; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), defaultTestPersistenceTimeout)
		resp, err := s.TestCluster.testBase.HistoryV2Mgr.GetHistoryTree(ctx, &persistence.GetHistoryTreeRequest{
			TreeID: run, ShardID: common.IntPtr(common.WorkflowIDToHistoryShard(id, s.TestClusterConfig.HistoryConfig.NumHistoryShards)), DomainName: s.DomainName})
		cancel()
		s.NoError(err)
		if len(resp.Branches) == 0 {
			return true
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false
}

// Retention: domains with retention 0 delete a run right after it closes, whichever side of the move the
// retention timer lands on. Two domains, flag toggled per domain: no executions row, no closed_executions
// row and no history may be left behind.
func (s *IntegrationSuite) TestWarmStorage_Retention_SLOW() {
	s.warmSkip()
	x, y := s.RandomizeStr("warm-ret-x"), s.RandomizeStr("warm-ret-y")
	s.warmRegisterDomain(x, 0)
	s.warmRegisterDomain(y, 0)
	xid := func() string {
		var id string
		s.warmIn(x, func() { id = s.warmDomainID() })
		return id
	}()
	type run struct{ domain, id, run string }
	var runs []run
	closeSome := func(n int) {
		for _, d := range []string{x, y} {
			s.warmIn(d, func() {
				for i := 0; i < n; i++ {
					id := "warm-ret-" + uuid.New()
					r := s.warmStart(id, nil)
					s.warmDecide(id, "bin-1", completeD())
					runs = append(runs, run{d, id, r})
				}
			})
		}
	}
	s.warmFlagDomains(false, map[string]bool{xid: true}) // X on, Y off
	closeSome(3)
	s.warmFlagDomains(false, nil) // X off too
	closeSome(3)
	s.warmFlagDomains(true, map[string]bool{xid: false}) // X off, Y on
	closeSome(3)
	s.warmFlagDomains(true, nil) // both on
	closeSome(3)
	for _, r := range runs {
		s.warmIn(r.domain, func() {
			s.True(s.warmHistoryDeleted(r.id, r.run), "history of %s/%s must be deleted", r.domain, r.id)
			var hot, warm int
			for i := 0; i < 40; i++ {
				hot, warm = s.warmRows(r.id, r.run)
				if hot == 0 && warm == 0 {
					break
				}
				time.Sleep(500 * time.Millisecond)
			}
			s.Equal([]int{0, 0}, []int{hot, warm}, "rows left behind for %s/%s (executions, closed_executions)", r.domain, r.id)
		})
	}
	s.warmFlagDomains(true, nil)
}

// Per-domain flag. X on / Y off at once; X off then on; Y on later; a domain created after the first move.
func (s *IntegrationSuite) TestWarmStorage_PerDomain_SLOW() {
	s.warmSkip()
	x, y := s.DomainName, s.ForeignDomainName
	xid := s.warmDomainID()
	var yid string
	s.warmIn(y, func() { yid = s.warmDomainID() })
	closeIn := func(d string) (id, run string) {
		s.warmIn(d, func() {
			id = "warm-pd-" + uuid.New()
			run = s.warmSetupDone(id, "bin-1", completeD())
		})
		return
	}
	expect := func(d, id, run string, hot, warm int) {
		s.warmIn(d, func() {
			if warm == 1 {
				s.warmWaitMoved(id, run)
			} else {
				time.Sleep(3 * time.Second)
			}
			h, w := s.warmRows(id, run)
			s.Equal([]int{hot, warm}, []int{h, w}, "%s/%s", d, id)
		})
	}
	readable := func(d, id, run string) {
		s.warmIn(d, func() {
			s.Equal("COMPLETED", s.warmStatus(id, run))
			s.NotEmpty(s.warmHistory(id, run))
			sig := s.warmSignal(id, run, "late")
			s.True(isCompletedErr(sig), "signal: %v", sig)
		})
	}

	// a. X on, Y off at the same time
	s.warmFlagDomains(false, map[string]bool{xid: true})
	xid1, xrun1 := closeIn(x)
	yid1, yrun1 := closeIn(y)
	expect(x, xid1, xrun1, 0, 1)
	expect(y, yid1, yrun1, 1, 0)
	readable(x, xid1, xrun1)
	readable(y, yid1, yrun1)
	// reset on Y behaves as on master; reset on warm X works
	for _, p := range []struct{ d, id, run string }{{x, xid1, xrun1}, {y, yid1, yrun1}} {
		s.warmIn(p.d, func() {
			eid := s.warmDecisionIDs(p.id, p.run, types.EventTypeDecisionTaskCompleted, true)
			out := s.warmResetOutcome(p.id, p.run, eid)
			s.Equal("reset=nil", out[0], p.d)
		})
	}

	// b. X off: warm runs stay readable, new closes stay hot
	s.warmFlagDomains(false, nil)
	readable(x, xid1, xrun1)
	xid2, xrun2 := closeIn(x)
	expect(x, xid2, xrun2, 1, 0)

	// c. X back on: new closes move, the hot closed run stays hot
	s.warmFlagDomains(false, map[string]bool{xid: true})
	xid3, xrun3 := closeIn(x)
	expect(x, xid3, xrun3, 0, 1)
	expect(x, xid2, xrun2, 1, 0)
	readable(x, xid2, xrun2)

	// d. Y on while X is on
	s.warmFlagDomains(false, map[string]bool{xid: true, yid: true})
	yid2, yrun2 := closeIn(y)
	expect(y, yid2, yrun2, 0, 1)
	expect(y, yid1, yrun1, 1, 0)
	readable(y, yid2, yrun2)

	// e. a domain created after the first move, flag off then on for it
	z := s.RandomizeStr("warm-pd-z")
	s.NoError(s.RegisterDomain(z, 1, types.ArchivalStatusDisabled, "", types.ArchivalStatusDisabled, "", nil))
	var zdom string
	s.warmIn(z, func() { zdom = s.warmDomainID() })
	zid1, zrun1 := closeIn(z)
	expect(z, zid1, zrun1, 1, 0)
	s.warmFlagDomains(false, map[string]bool{xid: true, yid: true, zdom: true})
	zid2, zrun2 := closeIn(z)
	expect(z, zid2, zrun2, 0, 1)
	readable(z, zid2, zrun2)
	readable(z, zid1, zrun1)
	s.warmFlagDomains(true, nil)
}

// Archival: the archiver reads the full state before the move; the archived history is complete and
// nothing is left behind.
func (s *IntegrationSuite) TestWarmStorage_Archival_SLOW() {
	s.warmSkip()
	s.True(s.TestCluster.archiverBase.metadata.GetHistoryConfig().ClusterConfiguredForArchival())
	s.warmFlag(true)
	s.warmIn(s.ArchivalDomainName, func() {
		for i := 0; i < 3; i++ {
			id := "warm-archival-" + uuid.New()
			run := s.warmSetupDone(id, "bin-1", completeD())
			exec := &types.WorkflowExecution{WorkflowID: id, RunID: run}
			s.True(s.isHistoryArchived(s.ArchivalDomainName, exec), "history must be archived")
			s.True(s.isHistoryDeleted(s.warmDomainID(), exec))
			var hot, warm int
			for j := 0; j < 40; j++ {
				if hot, warm = s.warmRows(id, run); hot == 0 && warm == 0 {
					break
				}
				time.Sleep(500 * time.Millisecond)
			}
			s.Equal([]int{0, 0}, []int{hot, warm}, "no rows may be left after archival")
			ctx, cancel := createContext()
			resp, err := s.Engine.GetWorkflowExecutionHistory(ctx, &types.GetWorkflowExecutionHistoryRequest{Domain: s.ArchivalDomainName, Execution: exec})
			cancel()
			s.NoError(err)
			s.True(resp.GetArchived())
			var got []string
			for _, e := range resp.History.Events {
				got = append(got, e.EventType.String())
			}
			s.Equal("WorkflowExecutionStarted,DecisionTaskScheduled,DecisionTaskStarted,DecisionTaskCompleted,WorkflowExecutionSignaled,DecisionTaskScheduled,DecisionTaskStarted,DecisionTaskCompleted,WorkflowExecutionCompleted", strings.Join(got, ","))
		}
	})
}

// The history scavenger deletes history without a mutable state. A warm run has none in executions, so the
// scavenger must still find it (fallback) and keep its history; a real orphan must still be deleted.
func (s *IntegrationSuite) TestWarmStorage_ScavengerAndScanner_SLOW() {
	s.warmSkip()
	s.warmFlag(true)
	id, orphanID := "warm-scav-"+uuid.New(), "warm-orphan-"+uuid.New()
	run := s.warmSetupDone(id, "bin-1", completeD())
	orphanRun := s.warmSetupDone(orphanID, "bin-1", completeD())
	s.warmSettle(true, [2]string{id, run}, [2]string{orphanID, orphanRun})
	domainID := s.warmDomainID()
	shard := common.WorkflowIDToHistoryShard(id, s.TestClusterConfig.HistoryConfig.NumHistoryShards)
	orphanShard := common.WorkflowIDToHistoryShard(orphanID, s.TestClusterConfig.HistoryConfig.NumHistoryShards)

	mgr := s.warmExecMgr()
	defer mgr.Close()
	ctx, cancel := createContext()
	defer cancel()
	// make the second run a genuine orphan: both rows gone, history kept
	s.NoError(mgr.DeleteWorkflowExecution(ctx, &persistence.DeleteWorkflowExecutionRequest{
		DomainID: domainID, WorkflowID: orphanID, RunID: orphanRun, ShardID: common.IntPtr(orphanShard)}))
	s.NoError(mgr.DeleteCurrentWorkflowExecution(ctx, &persistence.DeleteCurrentWorkflowExecutionRequest{
		DomainID: domainID, WorkflowID: orphanID, RunID: orphanRun, ShardID: common.IntPtr(orphanShard)}))
	hot, warm := s.warmRows(orphanID, orphanRun)
	s.Equal([]int{0, 0}, []int{hot, warm})

	ctrl := gomock.NewController(s.T())
	dc := cache.NewMockDomainCache(ctrl)
	dc.EXPECT().GetDomainName(gomock.Any()).Return(s.DomainName, nil).AnyTimes()
	scav := history.NewScavenger(s.TestCluster.testBase.HistoryV2Mgr, 100, s.HistoryClient, history.ScavengerHeartbeatDetails{},
		metrics.NewNoopMetricsClient(), log.NewNoop(), dynamicproperties.GetIntPropertyFn(0), dc)
	// Run heartbeats, which needs an activity context
	env := (&testsuite.WorkflowTestSuite{}).NewTestActivityEnvironment()
	env.RegisterActivityWithOptions(func(ctx context.Context) (history.ScavengerHeartbeatDetails, error) { return scav.Run(ctx) },
		activity.RegisterOptions{Name: "scavenge"})
	val, err := env.ExecuteActivity("scavenge")
	s.NoError(err)
	var hbd history.ScavengerHeartbeatDetails
	s.NoError(val.Get(&hbd))
	s.Zero(hbd.ErrorCount)
	s.False(s.warmHistoryDeletedQuick(id, run), "scavenger must keep the history of a warm run")
	s.True(s.warmHistoryDeleted(orphanID, orphanRun), "scavenger must delete the history of an orphan")
	s.NotEmpty(s.warmHistory(id, run))
	s.Equal([]int{0, 1}, func() []int { h, w := s.warmRows(id, run); return []int{h, w} }(), "scavenger must not touch the slim row")

	// scanner invariants over the warm run's current_executions row
	retryer := persistence.NewPersistenceRetryerWithShardID(mgr, s.TestCluster.testBase.HistoryV2Mgr, backoff.NewExponentialRetryPolicy(10*time.Millisecond), shard)
	dcache := dc
	cur := &entity.CurrentExecution{CurrentRunID: run, Execution: entity.Execution{ShardID: shard, DomainID: domainID, WorkflowID: id, RunID: run, State: persistence.WorkflowStateCompleted}}
	exists := invariant.NewConcreteExecutionExists(retryer, dcache)
	res := exists.Check(ctx, cur)
	s.Equal(invariant.CheckResultTypeHealthy, res.CheckResultType, "%+v", res)
	s.Equal(invariant.FixResultTypeSkipped, exists.Fix(ctx, cur).FixResultType)
	s.Equal([]int{0, 1}, func() []int { h, w := s.warmRows(id, run); return []int{h, w} }(), "fixer must not delete the warm run")
	s.Equal("COMPLETED", s.warmStatus(id, run))
}

func (s *IntegrationSuite) warmHistoryDeletedQuick(id, run string) bool {
	resp, err := s.TestCluster.testBase.HistoryV2Mgr.GetHistoryTree(context.Background(), &persistence.GetHistoryTreeRequest{
		TreeID: run, ShardID: common.IntPtr(common.WorkflowIDToHistoryShard(id, s.TestClusterConfig.HistoryConfig.NumHistoryShards)), DomainName: s.DomainName})
	s.NoError(err)
	return len(resp.Branches) == 0
}

// Flag on, off, on over moved runs, with the same reads in every state.
func (s *IntegrationSuite) TestWarmStorage_FlagOnOffOn_SLOW() {
	s.warmSkip()
	s.warmFlag(true)
	var all [][2]string
	one := func(wantWarm bool) {
		id := "warm-onoffon-" + uuid.New()
		run := s.warmSetupDone(id, "bin-1", completeD())
		if wantWarm {
			s.warmWaitMoved(id, run)
		} else {
			time.Sleep(3 * time.Second)
			h, w := s.warmRows(id, run)
			s.Equal([]int{1, 0}, []int{h, w})
		}
		all = append(all, [2]string{id, run})
	}
	one(true)
	s.warmFlag(false)
	one(false)
	s.warmFlag(true)
	one(true)
	var lines [][]string
	for _, p := range all {
		lines = append(lines, append(s.warmDescribeLines(p[0], p[1]), "types="+s.warmTypes(p[0], p[1])))
	}
	// the hot closed run and the warm ones describe the same, up to the rebuilt reset points
	s.Equal(lines[0], lines[1])
	s.Equal(lines[0], lines[2])
	h, w := s.warmRows(all[1][0], all[1][1])
	s.Equal([]int{1, 0}, []int{h, w}, "the run closed while the flag was off must stay hot (no migration)")
}

// A run that times out with a started activity still lists it in Describe, which a slim row cannot carry:
// it must stay hot with the flag on, and Describe must equal the flag-off one, pending activity included.
func (s *IntegrationSuite) TestWarmStorage_TimedOutWithStartedActivityStaysHot_SLOW() {
	s.warmBothWays(func(warm bool) []string {
		id := "warm-pendingact-" + uuid.New()
		run := s.warmStart(id, func(r *types.StartWorkflowExecutionRequest) {
			r.ExecutionStartToCloseTimeoutSeconds = common.Int32Ptr(4)
		})
		s.warmDecide(id, "bin-1", &types.Decision{
			DecisionType: types.DecisionTypeScheduleActivityTask.Ptr(),
			ScheduleActivityTaskDecisionAttributes: &types.ScheduleActivityTaskDecisionAttributes{
				ActivityID: "a1", ActivityType: &types.ActivityType{Name: "warm-act"}, TaskList: &types.TaskList{Name: id},
				ScheduleToCloseTimeoutSeconds: common.Int32Ptr(100), ScheduleToStartTimeoutSeconds: common.Int32Ptr(100),
				StartToCloseTimeoutSeconds: common.Int32Ptr(100), HeartbeatTimeoutSeconds: common.Int32Ptr(100)}})
		ctx, cancel := createContext()
		defer cancel()
		poll, err := s.Engine.PollForActivityTask(ctx, &types.PollForActivityTaskRequest{Domain: s.DomainName, TaskList: &types.TaskList{Name: id}, Identity: "warm"})
		s.NoError(err)
		s.NotEmpty(poll.TaskToken, "activity must be started")
		s.Equal("TIMED_OUT", s.warmWaitStatus(id, run, "TIMED_OUT"))
		// never moved, in either mode
		time.Sleep(5 * time.Second)
		hot, w := s.warmRows(id, run)
		s.Equal([]int{1, 0}, []int{hot, w}, "a closed run with an activity in flight must stay hot (warm=%v)", warm)
		shard := int32(common.WorkflowIDToHistoryShard(id, s.TestClusterConfig.HistoryConfig.NumHistoryShards))
		s.NoError(s.HistoryClient.CloseShard(ctx, &types.CloseShardRequest{ShardID: shard}))
		time.Sleep(time.Second)
		out := s.warmDescribeLines(id, run)
		for _, a := range s.warmDescribe(id, run).PendingActivities {
			out = append(out, fmt.Sprintf("activity=%s/%v/%v", a.ActivityID, a.State, a.ActivityType.Name))
		}
		return out
	})
}
