package errorinjectors

// Code generated by gowrap. DO NOT EDIT.
// template: ../../templates/errorinjectors.tmpl
// gowrap: http://github.com/hexdigest/gowrap

import (
	"context"

	"go.uber.org/yarpc"

	"github.com/uber/cadence/client/admin"
	"github.com/uber/cadence/common/errors"
	"github.com/uber/cadence/common/log"
	"github.com/uber/cadence/common/log/tag"
	"github.com/uber/cadence/common/types"
)

const (
	msgAdminInjectedFakeErr = "Injected fake admin client error"
)

// adminClient implements admin.Client interface instrumented with retries
type adminClient struct {
	client        admin.Client
	errorRate     float64
	logger        log.Logger
	fakeErrFn     func(float64) error
	forwardCallFn func(error) bool
}

// NewAdminClient creates a new instance of adminClient that injects error into every call with a given rate.
func NewAdminClient(client admin.Client, errorRate float64, logger log.Logger) admin.Client {
	return &adminClient{
		client:        client,
		errorRate:     errorRate,
		logger:        logger,
		fakeErrFn:     errors.GenerateFakeError,
		forwardCallFn: errors.ShouldForwardCall,
	}
}

func (c *adminClient) AddSearchAttribute(ctx context.Context, ap1 *types.AddSearchAttributeRequest, p1 ...yarpc.CallOption) (err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		err = c.client.AddSearchAttribute(ctx, ap1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgAdminInjectedFakeErr,
			tag.AdminClientOperationAddSearchAttribute,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *adminClient) CloseShard(ctx context.Context, cp1 *types.CloseShardRequest, p1 ...yarpc.CallOption) (err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		err = c.client.CloseShard(ctx, cp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgAdminInjectedFakeErr,
			tag.AdminClientOperationCloseShard,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *adminClient) CountDLQMessages(ctx context.Context, cp1 *types.CountDLQMessagesRequest, p1 ...yarpc.CallOption) (cp2 *types.CountDLQMessagesResponse, err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		cp2, err = c.client.CountDLQMessages(ctx, cp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgAdminInjectedFakeErr,
			tag.AdminClientOperationCountDLQMessages,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *adminClient) DeleteWorkflow(ctx context.Context, ap1 *types.AdminDeleteWorkflowRequest, p1 ...yarpc.CallOption) (ap2 *types.AdminDeleteWorkflowResponse, err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		ap2, err = c.client.DeleteWorkflow(ctx, ap1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgAdminInjectedFakeErr,
			tag.AdminClientOperationDeleteWorkflow,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *adminClient) DescribeCluster(ctx context.Context, p1 ...yarpc.CallOption) (dp1 *types.DescribeClusterResponse, err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		dp1, err = c.client.DescribeCluster(ctx, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgAdminInjectedFakeErr,
			tag.AdminClientOperationDescribeCluster,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *adminClient) DescribeHistoryHost(ctx context.Context, dp1 *types.DescribeHistoryHostRequest, p1 ...yarpc.CallOption) (dp2 *types.DescribeHistoryHostResponse, err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		dp2, err = c.client.DescribeHistoryHost(ctx, dp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgAdminInjectedFakeErr,
			tag.AdminClientOperationDescribeHistoryHost,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *adminClient) DescribeQueue(ctx context.Context, dp1 *types.DescribeQueueRequest, p1 ...yarpc.CallOption) (dp2 *types.DescribeQueueResponse, err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		dp2, err = c.client.DescribeQueue(ctx, dp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgAdminInjectedFakeErr,
			tag.AdminClientOperationDescribeQueue,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *adminClient) DescribeShardDistribution(ctx context.Context, dp1 *types.DescribeShardDistributionRequest, p1 ...yarpc.CallOption) (dp2 *types.DescribeShardDistributionResponse, err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		dp2, err = c.client.DescribeShardDistribution(ctx, dp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgAdminInjectedFakeErr,
			tag.AdminClientOperationDescribeShardDistribution,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *adminClient) DescribeWorkflowExecution(ctx context.Context, ap1 *types.AdminDescribeWorkflowExecutionRequest, p1 ...yarpc.CallOption) (ap2 *types.AdminDescribeWorkflowExecutionResponse, err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		ap2, err = c.client.DescribeWorkflowExecution(ctx, ap1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgAdminInjectedFakeErr,
			tag.AdminClientOperationDescribeWorkflowExecution,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *adminClient) GetDLQReplicationMessages(ctx context.Context, gp1 *types.GetDLQReplicationMessagesRequest, p1 ...yarpc.CallOption) (gp2 *types.GetDLQReplicationMessagesResponse, err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		gp2, err = c.client.GetDLQReplicationMessages(ctx, gp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgAdminInjectedFakeErr,
			tag.AdminClientOperationGetDLQReplicationMessages,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *adminClient) GetDomainAsyncWorkflowConfiguraton(ctx context.Context, request *types.GetDomainAsyncWorkflowConfiguratonRequest, opts ...yarpc.CallOption) (gp1 *types.GetDomainAsyncWorkflowConfiguratonResponse, err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		gp1, err = c.client.GetDomainAsyncWorkflowConfiguraton(ctx, request, opts...)
	}

	if fakeErr != nil {
		c.logger.Error(msgAdminInjectedFakeErr,
			tag.AdminClientOperationGetDomainAsyncWorkflowConfiguraton,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *adminClient) GetDomainIsolationGroups(ctx context.Context, request *types.GetDomainIsolationGroupsRequest, opts ...yarpc.CallOption) (gp1 *types.GetDomainIsolationGroupsResponse, err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		gp1, err = c.client.GetDomainIsolationGroups(ctx, request, opts...)
	}

	if fakeErr != nil {
		c.logger.Error(msgAdminInjectedFakeErr,
			tag.AdminClientOperationGetDomainIsolationGroups,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *adminClient) GetDomainReplicationMessages(ctx context.Context, gp1 *types.GetDomainReplicationMessagesRequest, p1 ...yarpc.CallOption) (gp2 *types.GetDomainReplicationMessagesResponse, err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		gp2, err = c.client.GetDomainReplicationMessages(ctx, gp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgAdminInjectedFakeErr,
			tag.AdminClientOperationGetDomainReplicationMessages,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *adminClient) GetDynamicConfig(ctx context.Context, gp1 *types.GetDynamicConfigRequest, p1 ...yarpc.CallOption) (gp2 *types.GetDynamicConfigResponse, err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		gp2, err = c.client.GetDynamicConfig(ctx, gp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgAdminInjectedFakeErr,
			tag.AdminClientOperationGetDynamicConfig,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *adminClient) GetGlobalIsolationGroups(ctx context.Context, request *types.GetGlobalIsolationGroupsRequest, opts ...yarpc.CallOption) (gp1 *types.GetGlobalIsolationGroupsResponse, err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		gp1, err = c.client.GetGlobalIsolationGroups(ctx, request, opts...)
	}

	if fakeErr != nil {
		c.logger.Error(msgAdminInjectedFakeErr,
			tag.AdminClientOperationGetGlobalIsolationGroups,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *adminClient) GetReplicationMessages(ctx context.Context, gp1 *types.GetReplicationMessagesRequest, p1 ...yarpc.CallOption) (gp2 *types.GetReplicationMessagesResponse, err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		gp2, err = c.client.GetReplicationMessages(ctx, gp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgAdminInjectedFakeErr,
			tag.AdminClientOperationGetReplicationMessages,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *adminClient) GetWorkflowExecutionRawHistoryV2(ctx context.Context, gp1 *types.GetWorkflowExecutionRawHistoryV2Request, p1 ...yarpc.CallOption) (gp2 *types.GetWorkflowExecutionRawHistoryV2Response, err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		gp2, err = c.client.GetWorkflowExecutionRawHistoryV2(ctx, gp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgAdminInjectedFakeErr,
			tag.AdminClientOperationGetWorkflowExecutionRawHistoryV2,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *adminClient) ListDynamicConfig(ctx context.Context, lp1 *types.ListDynamicConfigRequest, p1 ...yarpc.CallOption) (lp2 *types.ListDynamicConfigResponse, err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		lp2, err = c.client.ListDynamicConfig(ctx, lp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgAdminInjectedFakeErr,
			tag.AdminClientOperationListDynamicConfig,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *adminClient) MaintainCorruptWorkflow(ctx context.Context, ap1 *types.AdminMaintainWorkflowRequest, p1 ...yarpc.CallOption) (ap2 *types.AdminMaintainWorkflowResponse, err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		ap2, err = c.client.MaintainCorruptWorkflow(ctx, ap1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgAdminInjectedFakeErr,
			tag.AdminClientOperationMaintainCorruptWorkflow,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *adminClient) MergeDLQMessages(ctx context.Context, mp1 *types.MergeDLQMessagesRequest, p1 ...yarpc.CallOption) (mp2 *types.MergeDLQMessagesResponse, err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		mp2, err = c.client.MergeDLQMessages(ctx, mp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgAdminInjectedFakeErr,
			tag.AdminClientOperationMergeDLQMessages,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *adminClient) PurgeDLQMessages(ctx context.Context, pp1 *types.PurgeDLQMessagesRequest, p1 ...yarpc.CallOption) (err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		err = c.client.PurgeDLQMessages(ctx, pp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgAdminInjectedFakeErr,
			tag.AdminClientOperationPurgeDLQMessages,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *adminClient) ReadDLQMessages(ctx context.Context, rp1 *types.ReadDLQMessagesRequest, p1 ...yarpc.CallOption) (rp2 *types.ReadDLQMessagesResponse, err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		rp2, err = c.client.ReadDLQMessages(ctx, rp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgAdminInjectedFakeErr,
			tag.AdminClientOperationReadDLQMessages,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *adminClient) ReapplyEvents(ctx context.Context, rp1 *types.ReapplyEventsRequest, p1 ...yarpc.CallOption) (err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		err = c.client.ReapplyEvents(ctx, rp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgAdminInjectedFakeErr,
			tag.AdminClientOperationReapplyEvents,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *adminClient) RefreshWorkflowTasks(ctx context.Context, rp1 *types.RefreshWorkflowTasksRequest, p1 ...yarpc.CallOption) (err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		err = c.client.RefreshWorkflowTasks(ctx, rp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgAdminInjectedFakeErr,
			tag.AdminClientOperationRefreshWorkflowTasks,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *adminClient) RemoveTask(ctx context.Context, rp1 *types.RemoveTaskRequest, p1 ...yarpc.CallOption) (err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		err = c.client.RemoveTask(ctx, rp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgAdminInjectedFakeErr,
			tag.AdminClientOperationRemoveTask,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *adminClient) ResendReplicationTasks(ctx context.Context, rp1 *types.ResendReplicationTasksRequest, p1 ...yarpc.CallOption) (err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		err = c.client.ResendReplicationTasks(ctx, rp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgAdminInjectedFakeErr,
			tag.AdminClientOperationResendReplicationTasks,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *adminClient) ResetQueue(ctx context.Context, rp1 *types.ResetQueueRequest, p1 ...yarpc.CallOption) (err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		err = c.client.ResetQueue(ctx, rp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgAdminInjectedFakeErr,
			tag.AdminClientOperationResetQueue,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *adminClient) RestoreDynamicConfig(ctx context.Context, rp1 *types.RestoreDynamicConfigRequest, p1 ...yarpc.CallOption) (err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		err = c.client.RestoreDynamicConfig(ctx, rp1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgAdminInjectedFakeErr,
			tag.AdminClientOperationRestoreDynamicConfig,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *adminClient) UpdateDomainAsyncWorkflowConfiguraton(ctx context.Context, request *types.UpdateDomainAsyncWorkflowConfiguratonRequest, opts ...yarpc.CallOption) (up1 *types.UpdateDomainAsyncWorkflowConfiguratonResponse, err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		up1, err = c.client.UpdateDomainAsyncWorkflowConfiguraton(ctx, request, opts...)
	}

	if fakeErr != nil {
		c.logger.Error(msgAdminInjectedFakeErr,
			tag.AdminClientOperationUpdateDomainAsyncWorkflowConfiguraton,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *adminClient) UpdateDomainIsolationGroups(ctx context.Context, request *types.UpdateDomainIsolationGroupsRequest, opts ...yarpc.CallOption) (up1 *types.UpdateDomainIsolationGroupsResponse, err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		up1, err = c.client.UpdateDomainIsolationGroups(ctx, request, opts...)
	}

	if fakeErr != nil {
		c.logger.Error(msgAdminInjectedFakeErr,
			tag.AdminClientOperationUpdateDomainIsolationGroups,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *adminClient) UpdateDynamicConfig(ctx context.Context, up1 *types.UpdateDynamicConfigRequest, p1 ...yarpc.CallOption) (err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		err = c.client.UpdateDynamicConfig(ctx, up1, p1...)
	}

	if fakeErr != nil {
		c.logger.Error(msgAdminInjectedFakeErr,
			tag.AdminClientOperationUpdateDynamicConfig,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *adminClient) UpdateGlobalIsolationGroups(ctx context.Context, request *types.UpdateGlobalIsolationGroupsRequest, opts ...yarpc.CallOption) (up1 *types.UpdateGlobalIsolationGroupsResponse, err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		up1, err = c.client.UpdateGlobalIsolationGroups(ctx, request, opts...)
	}

	if fakeErr != nil {
		c.logger.Error(msgAdminInjectedFakeErr,
			tag.AdminClientOperationUpdateGlobalIsolationGroups,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}

func (c *adminClient) UpdateTaskListPartitionConfig(ctx context.Context, request *types.UpdateTaskListPartitionConfigRequest, opts ...yarpc.CallOption) (up1 *types.UpdateTaskListPartitionConfigResponse, err error) {
	fakeErr := c.fakeErrFn(c.errorRate)
	var forwardCall bool
	if forwardCall = c.forwardCallFn(fakeErr); forwardCall {
		up1, err = c.client.UpdateTaskListPartitionConfig(ctx, request, opts...)
	}

	if fakeErr != nil {
		c.logger.Error(msgAdminInjectedFakeErr,
			tag.AdminClientOperationUpdateTaskListPartitionConfig,
			tag.Error(fakeErr),
			tag.Bool(forwardCall),
			tag.ClientError(err),
		)
		err = fakeErr
		return
	}
	return
}
