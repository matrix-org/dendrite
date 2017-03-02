package api

import (
	"github.com/matrix-org/gomatrixserverlib"
)

// StateKeyTuple is a pair of an event type and state_key.
type StateKeyTuple struct {
	EventType     string
	EventStateKey string
}

// QueryLatestEventsAndStateRequest is a request to QueryLatestEventsAndState
type QueryLatestEventsAndStateRequest struct {
	// The roomID to query the latest events for.
	RoomID string
	// The state key tuples to fetch from the room current state.
	// If this list is empty or nil then no events are returned.
	StateToFetch []StateKeyTuple
}

// QueryLatestEventsAndStateResponse is a response to QueryLatestEventsAndState
type QueryLatestEventsAndStateResponse struct {
	// Copy of the request for debugging.
	QueryLatestEventsAndStateRequest
	// The latest events in the room.
	LatestEvents gomatrixserverlib.EventReference
	// The state events requested.
	StateEvents []gomatrixserverlib.Event
}

// RoomserverQueryAPI is used to query information from the room server.
type RoomserverQueryAPI interface {
	// Query the latest events and state for a room from the room server.
	QueryLatestEventsAndState(
		request *QueryLatestEventsAndStateRequest,
		response *QueryLatestEventsAndStateResponse,
	) error
}

// RPCServer is used to register a roomserver implementation with an RPC server.
type RPCServer interface {
	RegisterName(name string, rcvr interface{}) error
}

// RegisterRoomserverQueryAPI registers a RoomserverQueryAPI implementation with an RPC server.
func RegisterRoomserverQueryAPI(rpcServer RPCServer, roomserver RoomserverQueryAPI) error {
	return rpcServer.RegisterName("Roomserver", roomserver)
}

// RPCClient is used to invoke roomserver APIs on a remote server.
type RPCClient interface {
	Call(serviceMethod string, args interface{}, reply interface{}) error
}

// NewRoomserverQueryAPIFromClient creates a new query API from an RPC client.
func NewRoomserverQueryAPIFromClient(client RPCClient) RoomserverQueryAPI {
	return &remoteRoomserver{client}
}

type remoteRoomserver struct {
	client RPCClient
}

// QueryLatestEventsAndState implements RoomserverQueryAPI
func (r *remoteRoomserver) QueryLatestEventsAndState(
	request *QueryLatestEventsAndStateRequest,
	response *QueryLatestEventsAndStateResponse,
) error {
	return r.client.Call("Roomserver.QueryLatestEventsAndState", request, response)
}
