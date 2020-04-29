package query

import (
	"context"

	"github.com/matrix-org/dendrite/federationsender/api"
)

// InputJoinRequest implements api.FederationSenderQueryAPI
func (r *FederationSenderQueryAPI) InputJoinRequest(
	ctx context.Context,
	request *api.InputJoinRequest,
	response *api.InputJoinResponse,
) (err error) {
	return nil
}

// InputLeaveRequest implements api.FederationSenderQueryAPI
func (r *FederationSenderQueryAPI) InputLeaveRequest(
	ctx context.Context,
	request *api.InputLeaveRequest,
	response *api.InputLeaveResponse,
) (err error) {
	return nil
}
