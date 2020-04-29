package query

import (
	"context"

	"github.com/matrix-org/dendrite/federationsender/api"
)

// InputJoinRequest implements api.FederationSenderInternalAPI
func (r *FederationSenderInternalAPI) InputJoinRequest(
	ctx context.Context,
	request *api.InputJoinRequest,
	response *api.InputJoinResponse,
) (err error) {
	return nil
}

// InputLeaveRequest implements api.FederationSenderInternalAPI
func (r *FederationSenderInternalAPI) InputLeaveRequest(
	ctx context.Context,
	request *api.InputLeaveRequest,
	response *api.InputLeaveResponse,
) (err error) {
	return nil
}
