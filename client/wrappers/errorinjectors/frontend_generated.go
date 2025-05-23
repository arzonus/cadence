package errorinjectors

// Code generated by gowrap. DO NOT EDIT.
// template: ../../templates/errorinjectors.tmpl
// gowrap: http://github.com/hexdigest/gowrap

import (
	"context"

	"go.uber.org/yarpc"

	"github.com/uber/cadence/client/frontend"
	"github.com/uber/cadence/common/errors"
	"github.com/uber/cadence/common/log"
	"github.com/uber/cadence/common/log/tag"
	"github.com/uber/cadence/common/types"
)

const (
	msgFrontendInjectedFakeErr = "Injected fake frontend client error"
)

// frontendClient implements frontend.Client interface instrumented with retries
type frontendClient struct {
	client        frontend.Client
	errorRate     float64
	logger        log.Logger
	fakeErrFn     func(float64) error
	forwardCallFn func(error) bool
}

// NewFrontendClient creates a new instance of frontendClient that injects error into every call with a given rate.
func NewFrontendClient(client frontend.Client, errorRate float64, logger log.Logger) frontend.Client {
	return &frontendClient{
		client:        client,
		errorRate:     errorRate,
		logger:        logger,
		fakeErrFn:     errors.GenerateFakeError,
		forwardCallFn: errors.ShouldForwardCall,
	}
}

func (c *frontendClient) CountWorkflowExecutions(ctx context.Context, cp1 *types.CountWorkflowExecutionsRequest, p1 ...yarpc.CallOption) (cp2 *types.CountWorkflowExecutionsResponse, err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		cp2, err = c.client.CountWorkflowExecutions(ctx, cp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgFrontendInjectedFakeErr,
			tag.FrontendClientOperationCountWorkflowExecutions,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *frontendClient) DeleteDomain(ctx context.Context, dp1 *types.DeleteDomainRequest, p1 ...yarpc.CallOption) (err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		err = c.client.DeleteDomain(ctx, dp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgFrontendInjectedFakeErr,
			tag.FrontendClientOperationDeleteDomain,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *frontendClient) DeprecateDomain(ctx context.Context, dp1 *types.DeprecateDomainRequest, p1 ...yarpc.CallOption) (err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		err = c.client.DeprecateDomain(ctx, dp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgFrontendInjectedFakeErr,
			tag.FrontendClientOperationDeprecateDomain,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *frontendClient) DescribeDomain(ctx context.Context, dp1 *types.DescribeDomainRequest, p1 ...yarpc.CallOption) (dp2 *types.DescribeDomainResponse, err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		dp2, err = c.client.DescribeDomain(ctx, dp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgFrontendInjectedFakeErr,
			tag.FrontendClientOperationDescribeDomain,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *frontendClient) DescribeTaskList(ctx context.Context, dp1 *types.DescribeTaskListRequest, p1 ...yarpc.CallOption) (dp2 *types.DescribeTaskListResponse, err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		dp2, err = c.client.DescribeTaskList(ctx, dp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgFrontendInjectedFakeErr,
			tag.FrontendClientOperationDescribeTaskList,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *frontendClient) DescribeWorkflowExecution(ctx context.Context, dp1 *types.DescribeWorkflowExecutionRequest, p1 ...yarpc.CallOption) (dp2 *types.DescribeWorkflowExecutionResponse, err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		dp2, err = c.client.DescribeWorkflowExecution(ctx, dp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgFrontendInjectedFakeErr,
			tag.FrontendClientOperationDescribeWorkflowExecution,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *frontendClient) DiagnoseWorkflowExecution(ctx context.Context, dp1 *types.DiagnoseWorkflowExecutionRequest, p1 ...yarpc.CallOption) (dp2 *types.DiagnoseWorkflowExecutionResponse, err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		dp2, err = c.client.DiagnoseWorkflowExecution(ctx, dp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgFrontendInjectedFakeErr,
			tag.FrontendClientOperationDiagnoseWorkflowExecution,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *frontendClient) GetClusterInfo(ctx context.Context, p1 ...yarpc.CallOption) (cp1 *types.ClusterInfo, err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		cp1, err = c.client.GetClusterInfo(ctx, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgFrontendInjectedFakeErr,
			tag.FrontendClientOperationGetClusterInfo,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *frontendClient) GetSearchAttributes(ctx context.Context, p1 ...yarpc.CallOption) (gp1 *types.GetSearchAttributesResponse, err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		gp1, err = c.client.GetSearchAttributes(ctx, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgFrontendInjectedFakeErr,
			tag.FrontendClientOperationGetSearchAttributes,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *frontendClient) GetTaskListsByDomain(ctx context.Context, gp1 *types.GetTaskListsByDomainRequest, p1 ...yarpc.CallOption) (gp2 *types.GetTaskListsByDomainResponse, err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		gp2, err = c.client.GetTaskListsByDomain(ctx, gp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgFrontendInjectedFakeErr,
			tag.FrontendClientOperationGetTaskListsByDomain,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *frontendClient) GetWorkflowExecutionHistory(ctx context.Context, gp1 *types.GetWorkflowExecutionHistoryRequest, p1 ...yarpc.CallOption) (gp2 *types.GetWorkflowExecutionHistoryResponse, err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		gp2, err = c.client.GetWorkflowExecutionHistory(ctx, gp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgFrontendInjectedFakeErr,
			tag.FrontendClientOperationGetWorkflowExecutionHistory,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *frontendClient) ListArchivedWorkflowExecutions(ctx context.Context, lp1 *types.ListArchivedWorkflowExecutionsRequest, p1 ...yarpc.CallOption) (lp2 *types.ListArchivedWorkflowExecutionsResponse, err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		lp2, err = c.client.ListArchivedWorkflowExecutions(ctx, lp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgFrontendInjectedFakeErr,
			tag.FrontendClientOperationListArchivedWorkflowExecutions,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *frontendClient) ListClosedWorkflowExecutions(ctx context.Context, lp1 *types.ListClosedWorkflowExecutionsRequest, p1 ...yarpc.CallOption) (lp2 *types.ListClosedWorkflowExecutionsResponse, err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		lp2, err = c.client.ListClosedWorkflowExecutions(ctx, lp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgFrontendInjectedFakeErr,
			tag.FrontendClientOperationListClosedWorkflowExecutions,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *frontendClient) ListDomains(ctx context.Context, lp1 *types.ListDomainsRequest, p1 ...yarpc.CallOption) (lp2 *types.ListDomainsResponse, err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		lp2, err = c.client.ListDomains(ctx, lp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgFrontendInjectedFakeErr,
			tag.FrontendClientOperationListDomains,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *frontendClient) ListOpenWorkflowExecutions(ctx context.Context, lp1 *types.ListOpenWorkflowExecutionsRequest, p1 ...yarpc.CallOption) (lp2 *types.ListOpenWorkflowExecutionsResponse, err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		lp2, err = c.client.ListOpenWorkflowExecutions(ctx, lp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgFrontendInjectedFakeErr,
			tag.FrontendClientOperationListOpenWorkflowExecutions,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *frontendClient) ListTaskListPartitions(ctx context.Context, lp1 *types.ListTaskListPartitionsRequest, p1 ...yarpc.CallOption) (lp2 *types.ListTaskListPartitionsResponse, err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		lp2, err = c.client.ListTaskListPartitions(ctx, lp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgFrontendInjectedFakeErr,
			tag.FrontendClientOperationListTaskListPartitions,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *frontendClient) ListWorkflowExecutions(ctx context.Context, lp1 *types.ListWorkflowExecutionsRequest, p1 ...yarpc.CallOption) (lp2 *types.ListWorkflowExecutionsResponse, err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		lp2, err = c.client.ListWorkflowExecutions(ctx, lp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgFrontendInjectedFakeErr,
			tag.FrontendClientOperationListWorkflowExecutions,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *frontendClient) PollForActivityTask(ctx context.Context, pp1 *types.PollForActivityTaskRequest, p1 ...yarpc.CallOption) (pp2 *types.PollForActivityTaskResponse, err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		pp2, err = c.client.PollForActivityTask(ctx, pp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgFrontendInjectedFakeErr,
			tag.FrontendClientOperationPollForActivityTask,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *frontendClient) PollForDecisionTask(ctx context.Context, pp1 *types.PollForDecisionTaskRequest, p1 ...yarpc.CallOption) (pp2 *types.PollForDecisionTaskResponse, err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		pp2, err = c.client.PollForDecisionTask(ctx, pp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgFrontendInjectedFakeErr,
			tag.FrontendClientOperationPollForDecisionTask,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *frontendClient) QueryWorkflow(ctx context.Context, qp1 *types.QueryWorkflowRequest, p1 ...yarpc.CallOption) (qp2 *types.QueryWorkflowResponse, err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		qp2, err = c.client.QueryWorkflow(ctx, qp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgFrontendInjectedFakeErr,
			tag.FrontendClientOperationQueryWorkflow,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *frontendClient) RecordActivityTaskHeartbeat(ctx context.Context, rp1 *types.RecordActivityTaskHeartbeatRequest, p1 ...yarpc.CallOption) (rp2 *types.RecordActivityTaskHeartbeatResponse, err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		rp2, err = c.client.RecordActivityTaskHeartbeat(ctx, rp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgFrontendInjectedFakeErr,
			tag.FrontendClientOperationRecordActivityTaskHeartbeat,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *frontendClient) RecordActivityTaskHeartbeatByID(ctx context.Context, rp1 *types.RecordActivityTaskHeartbeatByIDRequest, p1 ...yarpc.CallOption) (rp2 *types.RecordActivityTaskHeartbeatResponse, err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		rp2, err = c.client.RecordActivityTaskHeartbeatByID(ctx, rp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgFrontendInjectedFakeErr,
			tag.FrontendClientOperationRecordActivityTaskHeartbeatByID,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *frontendClient) RefreshWorkflowTasks(ctx context.Context, rp1 *types.RefreshWorkflowTasksRequest, p1 ...yarpc.CallOption) (err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		err = c.client.RefreshWorkflowTasks(ctx, rp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgFrontendInjectedFakeErr,
			tag.FrontendClientOperationRefreshWorkflowTasks,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *frontendClient) RegisterDomain(ctx context.Context, rp1 *types.RegisterDomainRequest, p1 ...yarpc.CallOption) (err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		err = c.client.RegisterDomain(ctx, rp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgFrontendInjectedFakeErr,
			tag.FrontendClientOperationRegisterDomain,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *frontendClient) RequestCancelWorkflowExecution(ctx context.Context, rp1 *types.RequestCancelWorkflowExecutionRequest, p1 ...yarpc.CallOption) (err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		err = c.client.RequestCancelWorkflowExecution(ctx, rp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgFrontendInjectedFakeErr,
			tag.FrontendClientOperationRequestCancelWorkflowExecution,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *frontendClient) ResetStickyTaskList(ctx context.Context, rp1 *types.ResetStickyTaskListRequest, p1 ...yarpc.CallOption) (rp2 *types.ResetStickyTaskListResponse, err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		rp2, err = c.client.ResetStickyTaskList(ctx, rp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgFrontendInjectedFakeErr,
			tag.FrontendClientOperationResetStickyTaskList,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *frontendClient) ResetWorkflowExecution(ctx context.Context, rp1 *types.ResetWorkflowExecutionRequest, p1 ...yarpc.CallOption) (rp2 *types.ResetWorkflowExecutionResponse, err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		rp2, err = c.client.ResetWorkflowExecution(ctx, rp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgFrontendInjectedFakeErr,
			tag.FrontendClientOperationResetWorkflowExecution,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *frontendClient) RespondActivityTaskCanceled(ctx context.Context, rp1 *types.RespondActivityTaskCanceledRequest, p1 ...yarpc.CallOption) (err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		err = c.client.RespondActivityTaskCanceled(ctx, rp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgFrontendInjectedFakeErr,
			tag.FrontendClientOperationRespondActivityTaskCanceled,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *frontendClient) RespondActivityTaskCanceledByID(ctx context.Context, rp1 *types.RespondActivityTaskCanceledByIDRequest, p1 ...yarpc.CallOption) (err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		err = c.client.RespondActivityTaskCanceledByID(ctx, rp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgFrontendInjectedFakeErr,
			tag.FrontendClientOperationRespondActivityTaskCanceledByID,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *frontendClient) RespondActivityTaskCompleted(ctx context.Context, rp1 *types.RespondActivityTaskCompletedRequest, p1 ...yarpc.CallOption) (err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		err = c.client.RespondActivityTaskCompleted(ctx, rp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgFrontendInjectedFakeErr,
			tag.FrontendClientOperationRespondActivityTaskCompleted,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *frontendClient) RespondActivityTaskCompletedByID(ctx context.Context, rp1 *types.RespondActivityTaskCompletedByIDRequest, p1 ...yarpc.CallOption) (err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		err = c.client.RespondActivityTaskCompletedByID(ctx, rp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgFrontendInjectedFakeErr,
			tag.FrontendClientOperationRespondActivityTaskCompletedByID,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *frontendClient) RespondActivityTaskFailed(ctx context.Context, rp1 *types.RespondActivityTaskFailedRequest, p1 ...yarpc.CallOption) (err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		err = c.client.RespondActivityTaskFailed(ctx, rp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgFrontendInjectedFakeErr,
			tag.FrontendClientOperationRespondActivityTaskFailed,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *frontendClient) RespondActivityTaskFailedByID(ctx context.Context, rp1 *types.RespondActivityTaskFailedByIDRequest, p1 ...yarpc.CallOption) (err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		err = c.client.RespondActivityTaskFailedByID(ctx, rp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgFrontendInjectedFakeErr,
			tag.FrontendClientOperationRespondActivityTaskFailedByID,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *frontendClient) RespondDecisionTaskCompleted(ctx context.Context, rp1 *types.RespondDecisionTaskCompletedRequest, p1 ...yarpc.CallOption) (rp2 *types.RespondDecisionTaskCompletedResponse, err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		rp2, err = c.client.RespondDecisionTaskCompleted(ctx, rp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgFrontendInjectedFakeErr,
			tag.FrontendClientOperationRespondDecisionTaskCompleted,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *frontendClient) RespondDecisionTaskFailed(ctx context.Context, rp1 *types.RespondDecisionTaskFailedRequest, p1 ...yarpc.CallOption) (err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		err = c.client.RespondDecisionTaskFailed(ctx, rp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgFrontendInjectedFakeErr,
			tag.FrontendClientOperationRespondDecisionTaskFailed,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *frontendClient) RespondQueryTaskCompleted(ctx context.Context, rp1 *types.RespondQueryTaskCompletedRequest, p1 ...yarpc.CallOption) (err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		err = c.client.RespondQueryTaskCompleted(ctx, rp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgFrontendInjectedFakeErr,
			tag.FrontendClientOperationRespondQueryTaskCompleted,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *frontendClient) RestartWorkflowExecution(ctx context.Context, rp1 *types.RestartWorkflowExecutionRequest, p1 ...yarpc.CallOption) (rp2 *types.RestartWorkflowExecutionResponse, err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		rp2, err = c.client.RestartWorkflowExecution(ctx, rp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgFrontendInjectedFakeErr,
			tag.FrontendClientOperationRestartWorkflowExecution,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *frontendClient) ScanWorkflowExecutions(ctx context.Context, lp1 *types.ListWorkflowExecutionsRequest, p1 ...yarpc.CallOption) (lp2 *types.ListWorkflowExecutionsResponse, err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		lp2, err = c.client.ScanWorkflowExecutions(ctx, lp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgFrontendInjectedFakeErr,
			tag.FrontendClientOperationScanWorkflowExecutions,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *frontendClient) SignalWithStartWorkflowExecution(ctx context.Context, sp1 *types.SignalWithStartWorkflowExecutionRequest, p1 ...yarpc.CallOption) (sp2 *types.StartWorkflowExecutionResponse, err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		sp2, err = c.client.SignalWithStartWorkflowExecution(ctx, sp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgFrontendInjectedFakeErr,
			tag.FrontendClientOperationSignalWithStartWorkflowExecution,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *frontendClient) SignalWithStartWorkflowExecutionAsync(ctx context.Context, sp1 *types.SignalWithStartWorkflowExecutionAsyncRequest, p1 ...yarpc.CallOption) (sp2 *types.SignalWithStartWorkflowExecutionAsyncResponse, err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		sp2, err = c.client.SignalWithStartWorkflowExecutionAsync(ctx, sp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgFrontendInjectedFakeErr,
			tag.FrontendClientOperationSignalWithStartWorkflowExecutionAsync,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *frontendClient) SignalWorkflowExecution(ctx context.Context, sp1 *types.SignalWorkflowExecutionRequest, p1 ...yarpc.CallOption) (err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		err = c.client.SignalWorkflowExecution(ctx, sp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgFrontendInjectedFakeErr,
			tag.FrontendClientOperationSignalWorkflowExecution,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *frontendClient) StartWorkflowExecution(ctx context.Context, sp1 *types.StartWorkflowExecutionRequest, p1 ...yarpc.CallOption) (sp2 *types.StartWorkflowExecutionResponse, err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		sp2, err = c.client.StartWorkflowExecution(ctx, sp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgFrontendInjectedFakeErr,
			tag.FrontendClientOperationStartWorkflowExecution,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *frontendClient) StartWorkflowExecutionAsync(ctx context.Context, sp1 *types.StartWorkflowExecutionAsyncRequest, p1 ...yarpc.CallOption) (sp2 *types.StartWorkflowExecutionAsyncResponse, err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		sp2, err = c.client.StartWorkflowExecutionAsync(ctx, sp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgFrontendInjectedFakeErr,
			tag.FrontendClientOperationStartWorkflowExecutionAsync,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *frontendClient) TerminateWorkflowExecution(ctx context.Context, tp1 *types.TerminateWorkflowExecutionRequest, p1 ...yarpc.CallOption) (err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		err = c.client.TerminateWorkflowExecution(ctx, tp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgFrontendInjectedFakeErr,
			tag.FrontendClientOperationTerminateWorkflowExecution,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *frontendClient) UpdateDomain(ctx context.Context, up1 *types.UpdateDomainRequest, p1 ...yarpc.CallOption) (up2 *types.UpdateDomainResponse, err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		up2, err = c.client.UpdateDomain(ctx, up1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgFrontendInjectedFakeErr,
			tag.FrontendClientOperationUpdateDomain,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}
