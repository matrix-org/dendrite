package routing

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/matrix-org/dendrite/clientapi/producers"
	eduAPI "github.com/matrix-org/dendrite/eduserver/api"
	fsAPI "github.com/matrix-org/dendrite/federationsender/api"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"
)

const (
	testOrigin      = gomatrixserverlib.ServerName("kaer.morhen")
	testDestination = gomatrixserverlib.ServerName("white.orchard")
)

var (
	testRoomVersion = gomatrixserverlib.RoomVersionV1
	testData        = []json.RawMessage{
		[]byte(`{"auth_events":[],"content":{"creator":"@userid:kaer.morhen"},"depth":0,"event_id":"$0ok8ynDp7kjc95e3:kaer.morhen","hashes":{"sha256":"17kPoH+h0Dk4Omn7Sus0qMb6+oGcf+CZFEgDhv7UKWs"},"origin":"kaer.morhen","origin_server_ts":0,"prev_events":[],"prev_state":[],"room_id":"!roomid:kaer.morhen","sender":"@userid:kaer.morhen","signatures":{"kaer.morhen":{"ed25519:auto":"jP4a04f5/F10Pw95FPpdCyKAO44JOwUQ/MZOOeA/RTU1Dn+AHPMzGSaZnuGjRr/xQuADt+I3ctb5ZQfLKNzHDw"}},"state_key":"","type":"m.room.create"}`),
		[]byte(`{"auth_events":[["$0ok8ynDp7kjc95e3:kaer.morhen",{"sha256":"sWCi6Ckp9rDimQON+MrUlNRkyfZ2tjbPbWfg2NMB18Q"}]],"content":{"membership":"join"},"depth":1,"event_id":"$LEwEu0kxrtu5fOiS:kaer.morhen","hashes":{"sha256":"B7M88PhXf3vd1LaFtjQutFu4x/w7fHD28XKZ4sAsJTo"},"origin":"kaer.morhen","origin_server_ts":0,"prev_events":[["$0ok8ynDp7kjc95e3:kaer.morhen",{"sha256":"sWCi6Ckp9rDimQON+MrUlNRkyfZ2tjbPbWfg2NMB18Q"}]],"prev_state":[],"room_id":"!roomid:kaer.morhen","sender":"@userid:kaer.morhen","signatures":{"kaer.morhen":{"ed25519:auto":"p2vqmuJn7ZBRImctSaKbXCAxCcBlIjPH9JHte1ouIUGy84gpu4eLipOvSBCLL26hXfC0Zrm4WUto6Hr+ohdrCg"}},"state_key":"@userid:kaer.morhen","type":"m.room.member"}`),
		[]byte(`{"auth_events":[["$0ok8ynDp7kjc95e3:kaer.morhen",{"sha256":"sWCi6Ckp9rDimQON+MrUlNRkyfZ2tjbPbWfg2NMB18Q"}],["$LEwEu0kxrtu5fOiS:kaer.morhen",{"sha256":"1aKajq6DWHru1R1HJjvdWMEavkJJHGaTmPvfuERUXaA"}]],"content":{"join_rule":"public"},"depth":2,"event_id":"$SMHlqUrNhhBBRLeN:kaer.morhen","hashes":{"sha256":"vIuJQvmMjrGxshAkj1SXe0C4RqvMbv4ZADDw9pFCWqQ"},"origin":"kaer.morhen","origin_server_ts":0,"prev_events":[["$LEwEu0kxrtu5fOiS:kaer.morhen",{"sha256":"1aKajq6DWHru1R1HJjvdWMEavkJJHGaTmPvfuERUXaA"}]],"prev_state":[],"room_id":"!roomid:kaer.morhen","sender":"@userid:kaer.morhen","signatures":{"kaer.morhen":{"ed25519:auto":"hBMsb3Qppo3RaqqAl4JyTgaiWEbW5hlckATky6PrHun+F3YM203TzG7w9clwuQU5F5pZoB1a6nw+to0hN90FAw"}},"state_key":"","type":"m.room.join_rules"}`),
		[]byte(`{"auth_events":[["$0ok8ynDp7kjc95e3:kaer.morhen",{"sha256":"sWCi6Ckp9rDimQON+MrUlNRkyfZ2tjbPbWfg2NMB18Q"}],["$LEwEu0kxrtu5fOiS:kaer.morhen",{"sha256":"1aKajq6DWHru1R1HJjvdWMEavkJJHGaTmPvfuERUXaA"}]],"content":{"history_visibility":"shared"},"depth":3,"event_id":"$6F1yGIbO0J7TM93h:kaer.morhen","hashes":{"sha256":"Mr23GKSlZW7UCCYLgOWawI2Sg6KIoMjUWO2TDenuOgw"},"origin":"kaer.morhen","origin_server_ts":0,"prev_events":[["$SMHlqUrNhhBBRLeN:kaer.morhen",{"sha256":"SylzE8U02I+6eyEHgL+FlU0L5YdqrVp8OOlxKS9VQW0"}]],"prev_state":[],"room_id":"!roomid:kaer.morhen","sender":"@userid:kaer.morhen","signatures":{"kaer.morhen":{"ed25519:auto":"sHLKrFI3hKGrEJfpMVZSDS3LvLasQsy50CTsOwru9XTVxgRsPo6wozNtRVjxo1J3Rk18RC9JppovmQ5VR5EcDw"}},"state_key":"","type":"m.room.history_visibility"}`),
		[]byte(`{"auth_events":[["$0ok8ynDp7kjc95e3:kaer.morhen",{"sha256":"sWCi6Ckp9rDimQON+MrUlNRkyfZ2tjbPbWfg2NMB18Q"}],["$LEwEu0kxrtu5fOiS:kaer.morhen",{"sha256":"1aKajq6DWHru1R1HJjvdWMEavkJJHGaTmPvfuERUXaA"}]],"content":{"ban":50,"events":null,"events_default":0,"invite":0,"kick":50,"redact":50,"state_default":50,"users":null,"users_default":0},"depth":4,"event_id":"$UKNe10XzYzG0TeA9:kaer.morhen","hashes":{"sha256":"ngbP3yja9U5dlckKerUs/fSOhtKxZMCVvsfhPURSS28"},"origin":"kaer.morhen","origin_server_ts":0,"prev_events":[["$6F1yGIbO0J7TM93h:kaer.morhen",{"sha256":"A4CucrKSoWX4IaJXhq02mBg1sxIyZEftbC+5p3fZAvk"}]],"prev_state":[],"room_id":"!roomid:kaer.morhen","sender":"@userid:kaer.morhen","signatures":{"kaer.morhen":{"ed25519:auto":"zOmwlP01QL3yFchzuR9WHvogOoBZA3oVtNIF3lM0ZfDnqlSYZB9sns27G/4HVq0k7alaK7ZE3oGoCrVnMkPNCw"}},"state_key":"","type":"m.room.power_levels"}`),
		// messages
		[]byte(`{"auth_events":[["$0ok8ynDp7kjc95e3:kaer.morhen",{"sha256":"sWCi6Ckp9rDimQON+MrUlNRkyfZ2tjbPbWfg2NMB18Q"}],["$LEwEu0kxrtu5fOiS:kaer.morhen",{"sha256":"1aKajq6DWHru1R1HJjvdWMEavkJJHGaTmPvfuERUXaA"}]],"content":{"body":"Test Message"},"depth":5,"event_id":"$gl2T9l3qm0kUbiIJ:kaer.morhen","hashes":{"sha256":"Qx3nRMHLDPSL5hBAzuX84FiSSP0K0Kju2iFoBWH4Za8"},"origin":"kaer.morhen","origin_server_ts":0,"prev_events":[["$UKNe10XzYzG0TeA9:kaer.morhen",{"sha256":"KtSRyMjt0ZSjsv2koixTRCxIRCGoOp6QrKscsW97XRo"}]],"room_id":"!roomid:kaer.morhen","sender":"@userid:kaer.morhen","signatures":{"kaer.morhen":{"ed25519:auto":"sqDgv3EG7ml5VREzmT9aZeBpS4gAPNIaIeJOwqjDhY0GPU/BcpX5wY4R7hYLrNe5cChgV+eFy/GWm1Zfg5FfDg"}},"type":"m.room.message"}`),
		[]byte(`{"auth_events":[["$0ok8ynDp7kjc95e3:kaer.morhen",{"sha256":"sWCi6Ckp9rDimQON+MrUlNRkyfZ2tjbPbWfg2NMB18Q"}],["$LEwEu0kxrtu5fOiS:kaer.morhen",{"sha256":"1aKajq6DWHru1R1HJjvdWMEavkJJHGaTmPvfuERUXaA"}]],"content":{"body":"Test Message"},"depth":6,"event_id":"$MYSbs8m4rEbsCWXD:kaer.morhen","hashes":{"sha256":"kgbYM7v4Ud2YaBsjBTolM4ySg6rHcJNYI6nWhMSdFUA"},"origin":"kaer.morhen","origin_server_ts":0,"prev_events":[["$gl2T9l3qm0kUbiIJ:kaer.morhen",{"sha256":"C/rD04h9wGxRdN2G/IBfrgoE1UovzLZ+uskwaKZ37/Q"}]],"room_id":"!roomid:kaer.morhen","sender":"@userid:kaer.morhen","signatures":{"kaer.morhen":{"ed25519:auto":"x0UoKh968jj/F5l1/R7Ew0T6CTKuew3PLNHASNxqck/bkNe8yYQiDHXRr+kZxObeqPZZTpaF1+EI+bLU9W8GDQ"}},"type":"m.room.message"}`),
		[]byte(`{"auth_events":[["$0ok8ynDp7kjc95e3:kaer.morhen",{"sha256":"sWCi6Ckp9rDimQON+MrUlNRkyfZ2tjbPbWfg2NMB18Q"}],["$LEwEu0kxrtu5fOiS:kaer.morhen",{"sha256":"1aKajq6DWHru1R1HJjvdWMEavkJJHGaTmPvfuERUXaA"}]],"content":{"body":"Test Message"},"depth":7,"event_id":"$N5x9WJkl9ClPrAEg:kaer.morhen","hashes":{"sha256":"FWM8oz4yquTunRZ67qlW2gzPDzdWfBP6RPHXhK1I/x8"},"origin":"kaer.morhen","origin_server_ts":0,"prev_events":[["$MYSbs8m4rEbsCWXD:kaer.morhen",{"sha256":"fatqgW+SE8mb2wFn3UN+drmluoD4UJ/EcSrL6Ur9q1M"}]],"room_id":"!roomid:kaer.morhen","sender":"@userid:kaer.morhen","signatures":{"kaer.morhen":{"ed25519:auto":"Y+LX/xcyufoXMOIoqQBNOzy6lZfUGB1ffgXIrSugk6obMiyAsiRejHQN/pciZXsHKxMJLYRFAz4zSJoS/LGPAA"}},"type":"m.room.message"}`),
	}
	testEvents      = []gomatrixserverlib.HeaderedEvent{}
	testStateEvents = make(map[gomatrixserverlib.StateKeyTuple]gomatrixserverlib.HeaderedEvent)
)

func init() {
	for _, j := range testData {
		e, err := gomatrixserverlib.NewEventFromTrustedJSON(j, false, testRoomVersion)
		if err != nil {
			panic("cannot load test data: " + err.Error())
		}
		h := e.Headered(testRoomVersion)
		testEvents = append(testEvents, h)
		if e.StateKey() != nil {
			testStateEvents[gomatrixserverlib.StateKeyTuple{
				EventType: e.Type(),
				StateKey:  *e.StateKey(),
			}] = h
		}
	}
}

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

type testRoomserverAPI struct {
	inputRoomEvents       []api.InputRoomEvent
	queryStateAfterEvents func(*api.QueryStateAfterEventsRequest) api.QueryStateAfterEventsResponse
	queryEventsByID       func(req *api.QueryEventsByIDRequest) api.QueryEventsByIDResponse
}

func (t *testRoomserverAPI) SetFederationSenderAPI(fsAPI fsAPI.FederationSenderInternalAPI) {}

func (t *testRoomserverAPI) InputRoomEvents(
	ctx context.Context,
	request *api.InputRoomEventsRequest,
	response *api.InputRoomEventsResponse,
) error {
	t.inputRoomEvents = append(t.inputRoomEvents, request.InputRoomEvents...)
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
	response.RoomVersion = testRoomVersion
	response.QueryStateAfterEventsRequest = *request
	res := t.queryStateAfterEvents(request)
	response.PrevEventsExist = res.PrevEventsExist
	response.RoomExists = res.RoomExists
	response.StateEvents = res.StateEvents
	return nil
}

// Query a list of events by event ID.
func (t *testRoomserverAPI) QueryEventsByID(
	ctx context.Context,
	request *api.QueryEventsByIDRequest,
	response *api.QueryEventsByIDResponse,
) error {
	res := t.queryEventsByID(request)
	response.Events = res.Events
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
	response.RoomVersion = testRoomVersion
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

type txnFedClient struct {
	state    map[string]gomatrixserverlib.RespState    // event_id to response
	stateIDs map[string]gomatrixserverlib.RespStateIDs // event_id to response
	getEvent map[string]gomatrixserverlib.Transaction  // event_id to response
}

func (c *txnFedClient) LookupState(ctx context.Context, s gomatrixserverlib.ServerName, roomID string, eventID string, roomVersion gomatrixserverlib.RoomVersion) (
	res gomatrixserverlib.RespState, err error,
) {
	r, ok := c.state[eventID]
	if !ok {
		err = fmt.Errorf("txnFedClient: no /state for event %s", eventID)
		return
	}
	res = r
	return
}
func (c *txnFedClient) LookupStateIDs(ctx context.Context, s gomatrixserverlib.ServerName, roomID string, eventID string) (res gomatrixserverlib.RespStateIDs, err error) {
	r, ok := c.stateIDs[eventID]
	if !ok {
		err = fmt.Errorf("txnFedClient: no /state_ids for event %s", eventID)
		return
	}
	res = r
	return
}
func (c *txnFedClient) GetEvent(ctx context.Context, s gomatrixserverlib.ServerName, eventID string) (res gomatrixserverlib.Transaction, err error) {
	r, ok := c.getEvent[eventID]
	if !ok {
		err = fmt.Errorf("txnFedClient: no /event for event ID %s", eventID)
		return
	}
	res = r
	return
}

func mustCreateTransaction(rsAPI api.RoomserverInternalAPI, fedClient txnFederationClient, pdus []json.RawMessage) *txnReq {
	t := &txnReq{
		context:     context.Background(),
		rsAPI:       rsAPI,
		producer:    producers.NewRoomserverProducer(rsAPI),
		eduProducer: producers.NewEDUServerProducer(&testEDUProducer{}),
		keys:        &testNopJSONVerifier{},
		federation:  fedClient,
	}
	t.PDUs = pdus
	t.Origin = testOrigin
	t.TransactionID = gomatrixserverlib.TransactionID(fmt.Sprintf("%d", time.Now().UnixNano()))
	t.Destination = testDestination
	return t
}

func mustProcessTransaction(t *testing.T, txn *txnReq, pdusWithErrors []string) {
	res, err := txn.processTransaction()
	if err != nil {
		t.Errorf("txn.processTransaction returned an error: %s", err)
		return
	}
	if len(res.PDUs) != len(txn.PDUs) {
		t.Errorf("txn.processTransaction did not return results for all PDUs, got %d want %d", len(res.PDUs), len(txn.PDUs))
		return
	}
NextPDU:
	for eventID, result := range res.PDUs {
		if result.Error == "" {
			continue
		}
		for _, eventIDWantError := range pdusWithErrors {
			if eventID == eventIDWantError {
				break NextPDU
			}
		}
		t.Errorf("txn.processTransaction PDU %s returned an error %s", eventID, result.Error)
	}
}

func fromStateTuples(tuples []gomatrixserverlib.StateKeyTuple, omitTuples []gomatrixserverlib.StateKeyTuple) (result []gomatrixserverlib.HeaderedEvent) {
NextTuple:
	for _, t := range tuples {
		for _, o := range omitTuples {
			if t == o {
				break NextTuple
			}
		}
		h, ok := testStateEvents[t]
		if ok {
			result = append(result, h)
		}
	}
	return
}

func assertInputRoomEvents(t *testing.T, got []api.InputRoomEvent, want []gomatrixserverlib.HeaderedEvent) {
	if len(got) != len(want) {
		t.Errorf("wrong number of InputRoomEvents: got %d want %d", len(got), len(want))
		return
	}
	for i := range got {
		if got[i].Event.EventID() != want[i].EventID() {
			t.Errorf("InputRoomEvents[%d] got %s want %s", i, got[i].Event.EventID(), want[i].EventID())
		}
	}
}

// The purpose of this test is to check that receiving an event over federation for which we have the prev_events works correctly, and passes it on
// to the roomserver. It's the most basic test possible.
func TestBasicTransaction(t *testing.T) {
	rsAPI := &testRoomserverAPI{
		queryStateAfterEvents: func(req *api.QueryStateAfterEventsRequest) api.QueryStateAfterEventsResponse {
			return api.QueryStateAfterEventsResponse{
				PrevEventsExist: true,
				RoomExists:      true,
				StateEvents:     fromStateTuples(req.StateToFetch, nil),
			}
		},
	}
	pdus := []json.RawMessage{
		testData[len(testData)-1], // a message event
	}
	txn := mustCreateTransaction(rsAPI, &txnFedClient{}, pdus)
	mustProcessTransaction(t, txn, nil)
	assertInputRoomEvents(t, rsAPI.inputRoomEvents, []gomatrixserverlib.HeaderedEvent{testEvents[len(testEvents)-1]})
}

// The purpose of this test is to check that if the event received fails auth checks the transaction is failed.
func TestTransactionFailAuthChecks(t *testing.T) {
	rsAPI := &testRoomserverAPI{
		queryStateAfterEvents: func(req *api.QueryStateAfterEventsRequest) api.QueryStateAfterEventsResponse {
			return api.QueryStateAfterEventsResponse{
				PrevEventsExist: true,
				RoomExists:      true,
				// omit the create event so auth checks fail
				StateEvents: fromStateTuples(req.StateToFetch, []gomatrixserverlib.StateKeyTuple{
					{EventType: gomatrixserverlib.MRoomCreate, StateKey: ""},
				}),
			}
		},
	}
	pdus := []json.RawMessage{
		testData[len(testData)-1], // a message event
	}
	txn := mustCreateTransaction(rsAPI, &txnFedClient{}, pdus)
	mustProcessTransaction(t, txn, []string{
		// expect the event to have an error
		testEvents[len(testEvents)-1].EventID(),
	})
	assertInputRoomEvents(t, rsAPI.inputRoomEvents, nil) // expect no messages to be sent to the roomserver
}

// The purpose of this test is to check that when there are missing prev_events that state is fetched via /state_ids
// and /event and not /state. It works by setting PrevEventsExist=false in the roomserver query response, resulting in
// a call to /state_ids which returns the whole room state. It should attempt to fetch as many of these events from the
// roomserver FIRST, resulting in a call to QueryEventsByID. However, this will be missing the m.room.power_levels event which
// should then be requested via /event. The net result is that the transaction should succeed and there should be 2
// new events, first the m.room.power_levels event we were missing, then the transaction PDU.
func TestTransactionFetchMissingStateByStateIDs(t *testing.T) {
	missingStateEvent := testStateEvents[gomatrixserverlib.StateKeyTuple{
		EventType: gomatrixserverlib.MRoomPowerLevels,
		StateKey:  "",
	}]
	rsAPI := &testRoomserverAPI{
		queryStateAfterEvents: func(req *api.QueryStateAfterEventsRequest) api.QueryStateAfterEventsResponse {
			return api.QueryStateAfterEventsResponse{
				// setting this to false should trigger a call to /state_ids
				PrevEventsExist: false,
				RoomExists:      true,
				StateEvents:     nil,
			}
		},
		queryEventsByID: func(req *api.QueryEventsByIDRequest) api.QueryEventsByIDResponse {
			var res api.QueryEventsByIDResponse
			for _, wantEventID := range req.EventIDs {
				for _, ev := range testStateEvents {
					// roomserver is missing the power levels event
					if wantEventID == missingStateEvent.EventID() {
						continue
					}
					if ev.EventID() == wantEventID {
						res.Events = append(res.Events, ev)
					}
				}
			}
			res.QueryEventsByIDRequest = *req
			return res
		},
	}
	inputEvent := testEvents[len(testEvents)-1]
	var stateEventIDs []string
	for _, ev := range testStateEvents {
		stateEventIDs = append(stateEventIDs, ev.EventID())
	}
	cli := &txnFedClient{
		// /state_ids returns all the state events
		stateIDs: map[string]gomatrixserverlib.RespStateIDs{
			inputEvent.EventID(): gomatrixserverlib.RespStateIDs{
				StateEventIDs: stateEventIDs,
				AuthEventIDs:  stateEventIDs,
			},
		},
		// /event for the missing state event returns it
		getEvent: map[string]gomatrixserverlib.Transaction{
			missingStateEvent.EventID(): gomatrixserverlib.Transaction{
				PDUs: []json.RawMessage{
					missingStateEvent.JSON(),
				},
			},
		},
	}

	pdus := []json.RawMessage{
		testData[len(testData)-1], // a message event
	}
	txn := mustCreateTransaction(rsAPI, cli, pdus)
	mustProcessTransaction(t, txn, nil)
	assertInputRoomEvents(t, rsAPI.inputRoomEvents, []gomatrixserverlib.HeaderedEvent{missingStateEvent, inputEvent})
}

// The purpose of this test is to check that when there are missing prev_events and /state_ids fails, that we fallback to
// calling /state which returns the entire room state at that event. It works by setting PrevEventsExist=false in the
// roomserver query response, resulting in a call to /state_ids which fails (unset). It should then fetch via /state.
func TestTransactionFetchMissingStateByFallbackState(t *testing.T) {
	rsAPI := &testRoomserverAPI{
		queryStateAfterEvents: func(req *api.QueryStateAfterEventsRequest) api.QueryStateAfterEventsResponse {
			return api.QueryStateAfterEventsResponse{
				// setting this to false should trigger a call to /state_ids
				PrevEventsExist: false,
				RoomExists:      true,
				StateEvents:     nil,
			}
		},
	}
	inputEvent := testEvents[len(testEvents)-1]
	var stateEvents []gomatrixserverlib.HeaderedEvent
	for _, ev := range testStateEvents {
		stateEvents = append(stateEvents, ev)
	}
	cli := &txnFedClient{
		// /state_ids purposefully unset
		stateIDs: nil,
		// /state returns the state at that event (which is the current state)
		state: map[string]gomatrixserverlib.RespState{
			inputEvent.EventID(): gomatrixserverlib.RespState{
				AuthEvents:  gomatrixserverlib.UnwrapEventHeaders(stateEvents),
				StateEvents: gomatrixserverlib.UnwrapEventHeaders(stateEvents),
			},
		},
	}

	pdus := []json.RawMessage{
		testData[len(testData)-1], // a message event
	}
	txn := mustCreateTransaction(rsAPI, cli, pdus)
	mustProcessTransaction(t, txn, nil)
	// the roomserver should get all state events and the new input event
	// TODO: it should really be only giving the missing ones
	assertInputRoomEvents(t, rsAPI.inputRoomEvents, append(stateEvents, inputEvent))
}
