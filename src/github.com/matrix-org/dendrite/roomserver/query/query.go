package query

import (
	"fmt"
	"github.com/matrix-org/dendrite/roomserver/api"
)

// RoomserverQueryAPI is an implemenation of RoomserverQueryAPI
type RoomserverQueryAPI struct {
}

// QueryLatestEventsAndState implements api.RoomserverQueryAPI
func (r *RoomserverQueryAPI) QueryLatestEventsAndState(
	request *api.QueryLatestEventsAndStateRequest,
	response *api.QueryLatestEventsAndStateResponse,
) error {
	return fmt.Errorf("Not Implemented")
}
