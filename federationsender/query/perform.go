package query

import (
	"context"

	"github.com/matrix-org/dendrite/federationsender/api"
)

// PerformJoinRequest implements api.FederationSenderInternalAPI
func (r *FederationSenderInternalAPI) PerformJoinRequest(
	ctx context.Context,
	request *api.PerformJoinRequest,
	response *api.PerformJoinResponse,
) (err error) {
	return nil
}

// PerformLeaveRequest implements api.FederationSenderInternalAPI
func (r *FederationSenderInternalAPI) PerformLeaveRequest(
	ctx context.Context,
	request *api.PerformLeaveRequest,
	response *api.PerformLeaveResponse,
) (err error) {
	return nil
}
