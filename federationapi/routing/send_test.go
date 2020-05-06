package routing

import (
	"context"
	"testing"

	"github.com/matrix-org/dendrite/clientapi/producers"
	eduAPI "github.com/matrix-org/dendrite/eduserver/api"
	fsAPI "github.com/matrix-org/dendrite/federationsender/api"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"
)

type testNopJSONVerifier struct {
	// this verifier verifies nothing
}

func (t *testNopJSONVerifier) VerifyJSONs(ctx context.Context, requests []gomatrixserverlib.VerifyJSONRequest) ([]gomatrixserverlib.VerifyJSONResult, error) {
	result := make([]gomatrixserverlib.VerifyJSONResult, len(requests))
	return result, nil
}

type testEDUProducer struct {
	// this producer keeps track of calls to InputTypingEvent
	invocations []eduAPI.InputTypingEventRequest
}

func (p *testEDUProducer) InputTypingEvent(
	ctx context.Context,
	request *eduAPI.InputTypingEventRequest,
	response *eduAPI.InputTypingEventResponse,
) error {
	p.invocations = append(p.invocations, *request)
	return nil
}

type testRoomserverAPI struct{}

func (t *testRoomserverAPI) SetFederationSenderAPI(fsAPI fsAPI.FederationSenderInternalAPI) {}

func (t *testRoomserverAPI) InputRoomEvents(
	ctx context.Context,
	request *api.InputRoomEventsRequest,
	response *api.InputRoomEventsResponse,
) error {
	return nil
}

func (t *testRoomserverAPI) PerformJoin(
	ctx context.Context,
	req *api.PerformJoinRequest,
	res *api.PerformJoinResponse,
) error {
	return nil
}

func (t *testRoomserverAPI) PerformLeave(
	ctx context.Context,
	req *api.PerformLeaveRequest,
	res *api.PerformLeaveResponse,
) error {
	return nil
}

// Query the latest events and state for a room from the room server.
func (t *testRoomserverAPI) QueryLatestEventsAndState(
	ctx context.Context,
	request *api.QueryLatestEventsAndStateRequest,
	response *api.QueryLatestEventsAndStateResponse,
) error {
	return nil
}

// Query the state after a list of events in a room from the room server.
func (t *testRoomserverAPI) QueryStateAfterEvents(
	ctx context.Context,
	request *api.QueryStateAfterEventsRequest,
	response *api.QueryStateAfterEventsResponse,
) error {
	return nil
}

// Query a list of events by event ID.
func (t *testRoomserverAPI) QueryEventsByID(
	ctx context.Context,
	request *api.QueryEventsByIDRequest,
	response *api.QueryEventsByIDResponse,
) error {
	return nil
}

// Query the membership event for an user for a room.
func (t *testRoomserverAPI) QueryMembershipForUser(
	ctx context.Context,
	request *api.QueryMembershipForUserRequest,
	response *api.QueryMembershipForUserResponse,
) error {
	return nil
}

// Query a list of membership events for a room
func (t *testRoomserverAPI) QueryMembershipsForRoom(
	ctx context.Context,
	request *api.QueryMembershipsForRoomRequest,
	response *api.QueryMembershipsForRoomResponse,
) error {
	return nil
}

// Query a list of invite event senders for a user in a room.
func (t *testRoomserverAPI) QueryInvitesForUser(
	ctx context.Context,
	request *api.QueryInvitesForUserRequest,
	response *api.QueryInvitesForUserResponse,
) error {
	return nil
}

// Query whether a server is allowed to see an event
func (t *testRoomserverAPI) QueryServerAllowedToSeeEvent(
	ctx context.Context,
	request *api.QueryServerAllowedToSeeEventRequest,
	response *api.QueryServerAllowedToSeeEventResponse,
) error {
	return nil
}

// Query missing events for a room from roomserver
func (t *testRoomserverAPI) QueryMissingEvents(
	ctx context.Context,
	request *api.QueryMissingEventsRequest,
	response *api.QueryMissingEventsResponse,
) error {
	return nil
}

// Query to get state and auth chain for a (potentially hypothetical) event.
// Takes lists of PrevEventIDs and AuthEventsIDs and uses them to calculate
// the state and auth chain to return.
func (t *testRoomserverAPI) QueryStateAndAuthChain(
	ctx context.Context,
	request *api.QueryStateAndAuthChainRequest,
	response *api.QueryStateAndAuthChainResponse,
) error {
	return nil
}

// Query a given amount (or less) of events prior to a given set of events.
func (t *testRoomserverAPI) QueryBackfill(
	ctx context.Context,
	request *api.QueryBackfillRequest,
	response *api.QueryBackfillResponse,
) error {
	return nil
}

// Asks for the default room version as preferred by the server.
func (t *testRoomserverAPI) QueryRoomVersionCapabilities(
	ctx context.Context,
	request *api.QueryRoomVersionCapabilitiesRequest,
	response *api.QueryRoomVersionCapabilitiesResponse,
) error {
	return nil
}

// Asks for the room version for a given room.
func (t *testRoomserverAPI) QueryRoomVersionForRoom(
	ctx context.Context,
	request *api.QueryRoomVersionForRoomRequest,
	response *api.QueryRoomVersionForRoomResponse,
) error {
	return nil
}

// Set a room alias
func (t *testRoomserverAPI) SetRoomAlias(
	ctx context.Context,
	req *api.SetRoomAliasRequest,
	response *api.SetRoomAliasResponse,
) error {
	return nil
}

// Get the room ID for an alias
func (t *testRoomserverAPI) GetRoomIDForAlias(
	ctx context.Context,
	req *api.GetRoomIDForAliasRequest,
	response *api.GetRoomIDForAliasResponse,
) error {
	return nil
}

// Get all known aliases for a room ID
func (t *testRoomserverAPI) GetAliasesForRoomID(
	ctx context.Context,
	req *api.GetAliasesForRoomIDRequest,
	response *api.GetAliasesForRoomIDResponse,
) error {
	return nil
}

// Get the user ID of the creator of an alias
func (t *testRoomserverAPI) GetCreatorIDForAlias(
	ctx context.Context,
	req *api.GetCreatorIDForAliasRequest,
	response *api.GetCreatorIDForAliasResponse,
) error {
	return nil
}

// Remove a room alias
func (t *testRoomserverAPI) RemoveRoomAlias(
	ctx context.Context,
	req *api.RemoveRoomAliasRequest,
	response *api.RemoveRoomAliasResponse,
) error {
	return nil
}

type txnFedClient struct{}

func (c *txnFedClient) LookupState(ctx context.Context, s gomatrixserverlib.ServerName, roomID string, eventID string, roomVersion gomatrixserverlib.RoomVersion) (
	res gomatrixserverlib.RespState, err error,
) {
	return
}
func (c *txnFedClient) LookupStateIDs(ctx context.Context, s gomatrixserverlib.ServerName, roomID string, eventID string) (res gomatrixserverlib.RespStateIDs, err error) {
	return
}
func (c *txnFedClient) GetEvent(ctx context.Context, s gomatrixserverlib.ServerName, eventID string) (res gomatrixserverlib.Transaction, err error) {
	return
}

func mustCreateTransaction(t *testing.T, rsAPI api.RoomserverInternalAPI) *txnReq {
	return &txnReq{
		context:     context.Background(),
		rsAPI:       rsAPI,
		producer:    producers.NewRoomserverProducer(rsAPI),
		eduProducer: producers.NewEDUServerProducer(&testEDUProducer{}),
		keys:        &testNopJSONVerifier{},
		federation:  &txnFedClient{},
	}
}

func TestBasicTransaction(t *testing.T) {
	mustCreateTransaction(t, &testRoomserverAPI{})
}
