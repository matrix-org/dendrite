package routing

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	eduAPI "github.com/matrix-org/dendrite/eduserver/api"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/test"
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
	testEvents      = []*gomatrixserverlib.HeaderedEvent{}
	testStateEvents = make(map[gomatrixserverlib.StateKeyTuple]*gomatrixserverlib.HeaderedEvent)
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

func (p *testEDUProducer) InputSendToDeviceEvent(
	ctx context.Context,
	request *eduAPI.InputSendToDeviceEventRequest,
	response *eduAPI.InputSendToDeviceEventResponse,
) error {
	return nil
}

func (o *testEDUProducer) InputReceiptEvent(
	ctx context.Context,
	request *eduAPI.InputReceiptEventRequest,
	response *eduAPI.InputReceiptEventResponse,
) error {
	return nil
}

func (o *testEDUProducer) InputCrossSigningKeyUpdate(
	ctx context.Context,
	request *eduAPI.InputCrossSigningKeyUpdateRequest,
	response *eduAPI.InputCrossSigningKeyUpdateResponse,
) error {
	return nil
}

type testRoomserverAPI struct {
	api.RoomserverInternalAPITrace
	inputRoomEvents           []api.InputRoomEvent
	queryStateAfterEvents     func(*api.QueryStateAfterEventsRequest) api.QueryStateAfterEventsResponse
	queryEventsByID           func(req *api.QueryEventsByIDRequest) api.QueryEventsByIDResponse
	queryLatestEventsAndState func(*api.QueryLatestEventsAndStateRequest) api.QueryLatestEventsAndStateResponse
}

func (t *testRoomserverAPI) InputRoomEvents(
	ctx context.Context,
	request *api.InputRoomEventsRequest,
	response *api.InputRoomEventsResponse,
) {
	t.inputRoomEvents = append(t.inputRoomEvents, request.InputRoomEvents...)
	for _, ire := range request.InputRoomEvents {
		fmt.Println("InputRoomEvents: ", ire.Event.EventID())
	}
}

// Query the latest events and state for a room from the room server.
func (t *testRoomserverAPI) QueryLatestEventsAndState(
	ctx context.Context,
	request *api.QueryLatestEventsAndStateRequest,
	response *api.QueryLatestEventsAndStateResponse,
) error {
	r := t.queryLatestEventsAndState(request)
	response.RoomExists = r.RoomExists
	response.RoomVersion = testRoomVersion
	response.LatestEvents = r.LatestEvents
	response.StateEvents = r.StateEvents
	response.Depth = r.Depth
	return nil
}

// Query the state after a list of events in a room from the room server.
func (t *testRoomserverAPI) QueryStateAfterEvents(
	ctx context.Context,
	request *api.QueryStateAfterEventsRequest,
	response *api.QueryStateAfterEventsResponse,
) error {
	response.RoomVersion = testRoomVersion
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

// Query if a server is joined to a room
func (t *testRoomserverAPI) QueryServerJoinedToRoom(
	ctx context.Context,
	request *api.QueryServerJoinedToRoomRequest,
	response *api.QueryServerJoinedToRoomResponse,
) error {
	response.RoomExists = true
	response.IsInRoom = true
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

func (t *testRoomserverAPI) QueryServerBannedFromRoom(
	ctx context.Context, req *api.QueryServerBannedFromRoomRequest, res *api.QueryServerBannedFromRoomResponse,
) error {
	res.Banned = false
	return nil
}

type txnFedClient struct {
	state            map[string]gomatrixserverlib.RespState    // event_id to response
	stateIDs         map[string]gomatrixserverlib.RespStateIDs // event_id to response
	getEvent         map[string]gomatrixserverlib.Transaction  // event_id to response
	getMissingEvents func(gomatrixserverlib.MissingEvents) (res gomatrixserverlib.RespMissingEvents, err error)
}

func (c *txnFedClient) LookupState(ctx context.Context, s gomatrixserverlib.ServerName, roomID string, eventID string, roomVersion gomatrixserverlib.RoomVersion) (
	res gomatrixserverlib.RespState, err error,
) {
	fmt.Println("testFederationClient.LookupState", eventID)
	r, ok := c.state[eventID]
	if !ok {
		err = fmt.Errorf("txnFedClient: no /state for event %s", eventID)
		return
	}
	res = r
	return
}
func (c *txnFedClient) LookupStateIDs(ctx context.Context, s gomatrixserverlib.ServerName, roomID string, eventID string) (res gomatrixserverlib.RespStateIDs, err error) {
	fmt.Println("testFederationClient.LookupStateIDs", eventID)
	r, ok := c.stateIDs[eventID]
	if !ok {
		err = fmt.Errorf("txnFedClient: no /state_ids for event %s", eventID)
		return
	}
	res = r
	return
}
func (c *txnFedClient) GetEvent(ctx context.Context, s gomatrixserverlib.ServerName, eventID string) (res gomatrixserverlib.Transaction, err error) {
	fmt.Println("testFederationClient.GetEvent", eventID)
	r, ok := c.getEvent[eventID]
	if !ok {
		err = fmt.Errorf("txnFedClient: no /event for event ID %s", eventID)
		return
	}
	res = r
	return
}
func (c *txnFedClient) LookupMissingEvents(ctx context.Context, s gomatrixserverlib.ServerName, roomID string, missing gomatrixserverlib.MissingEvents,
	roomVersion gomatrixserverlib.RoomVersion) (res gomatrixserverlib.RespMissingEvents, err error) {
	return c.getMissingEvents(missing)
}

func mustCreateTransaction(rsAPI api.RoomserverInternalAPI, fedClient txnFederationClient, pdus []json.RawMessage) *txnReq {
	t := &txnReq{
		rsAPI:      rsAPI,
		eduAPI:     &testEDUProducer{},
		keys:       &test.NopJSONVerifier{},
		federation: fedClient,
		roomsMu:    internal.NewMutexByRoom(),
	}
	t.PDUs = pdus
	t.Origin = testOrigin
	t.TransactionID = gomatrixserverlib.TransactionID(fmt.Sprintf("%d", time.Now().UnixNano()))
	t.Destination = testDestination
	return t
}

func mustProcessTransaction(t *testing.T, txn *txnReq, pdusWithErrors []string) {
	res, err := txn.processTransaction(context.Background())
	if err != nil {
		t.Errorf("txn.processTransaction returned an error: %v", err)
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

/*
func fromStateTuples(tuples []gomatrixserverlib.StateKeyTuple, omitTuples []gomatrixserverlib.StateKeyTuple) (result []*gomatrixserverlib.HeaderedEvent) {
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
*/

func assertInputRoomEvents(t *testing.T, got []api.InputRoomEvent, want []*gomatrixserverlib.HeaderedEvent) {
	for _, g := range got {
		fmt.Println("GOT ", g.Event.EventID())
	}
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
	rsAPI := &testRoomserverAPI{}
	pdus := []json.RawMessage{
		testData[len(testData)-1], // a message event
	}
	txn := mustCreateTransaction(rsAPI, &txnFedClient{}, pdus)
	mustProcessTransaction(t, txn, nil)
	assertInputRoomEvents(t, rsAPI.inputRoomEvents, []*gomatrixserverlib.HeaderedEvent{testEvents[len(testEvents)-1]})
}

// The purpose of this test is to check that if the event received fails auth checks the event is still sent to the roomserver
// as it does the auth check.
func TestTransactionFailAuthChecks(t *testing.T) {
	rsAPI := &testRoomserverAPI{}
	pdus := []json.RawMessage{
		testData[len(testData)-1], // a message event
	}
	txn := mustCreateTransaction(rsAPI, &txnFedClient{}, pdus)
	mustProcessTransaction(t, txn, []string{})
	// expect message to be sent to the roomserver
	assertInputRoomEvents(t, rsAPI.inputRoomEvents, []*gomatrixserverlib.HeaderedEvent{testEvents[len(testEvents)-1]})
}

// The purpose of this test is to make sure that when an event is received for which we do not know the prev_events,
// we request them from /get_missing_events. It works by setting PrevEventsExist=false in the roomserver query response,
// resulting in a call to /get_missing_events which returns the missing prev event. Both events should be processed in
// topological order and sent to the roomserver.
/*
func TestTransactionFetchMissingPrevEvents(t *testing.T) {
	haveEvent := testEvents[len(testEvents)-3]
	prevEvent := testEvents[len(testEvents)-2]
	inputEvent := testEvents[len(testEvents)-1]

	var rsAPI *testRoomserverAPI // ref here so we can refer to inputRoomEvents inside these functions
	rsAPI = &testRoomserverAPI{
		queryEventsByID: func(req *api.QueryEventsByIDRequest) api.QueryEventsByIDResponse {
			res := api.QueryEventsByIDResponse{}
			for _, ev := range testEvents {
				for _, id := range req.EventIDs {
					if ev.EventID() == id {
						res.Events = append(res.Events, ev)
					}
				}
			}
			return res
		},
		queryStateAfterEvents: func(req *api.QueryStateAfterEventsRequest) api.QueryStateAfterEventsResponse {
			return api.QueryStateAfterEventsResponse{
				PrevEventsExist: true,
				StateEvents:     testEvents[:5],
			}
		},
		queryMissingAuthPrevEvents: func(req *api.QueryMissingAuthPrevEventsRequest) api.QueryMissingAuthPrevEventsResponse {
			missingPrevEvent := []string{"missing_prev_event"}
			if len(req.PrevEventIDs) == 1 {
				switch req.PrevEventIDs[0] {
				case haveEvent.EventID():
					missingPrevEvent = []string{}
				case prevEvent.EventID():
					// we only have this event if we've been send prevEvent
					if len(rsAPI.inputRoomEvents) == 1 && rsAPI.inputRoomEvents[0].Event.EventID() == prevEvent.EventID() {
						missingPrevEvent = []string{}
					}
				}
			}

			return api.QueryMissingAuthPrevEventsResponse{
				RoomExists:          true,
				MissingAuthEventIDs: []string{},
				MissingPrevEventIDs: missingPrevEvent,
			}
		},
		queryLatestEventsAndState: func(req *api.QueryLatestEventsAndStateRequest) api.QueryLatestEventsAndStateResponse {
			return api.QueryLatestEventsAndStateResponse{
				RoomExists: true,
				Depth:      haveEvent.Depth(),
				LatestEvents: []gomatrixserverlib.EventReference{
					haveEvent.EventReference(),
				},
				StateEvents: fromStateTuples(req.StateToFetch, nil),
			}
		},
	}

	cli := &txnFedClient{
		getMissingEvents: func(missing gomatrixserverlib.MissingEvents) (res gomatrixserverlib.RespMissingEvents, err error) {
			if !reflect.DeepEqual(missing.EarliestEvents, []string{haveEvent.EventID()}) {
				t.Errorf("call to /get_missing_events wrong earliest events: got %v want %v", missing.EarliestEvents, haveEvent.EventID())
			}
			if !reflect.DeepEqual(missing.LatestEvents, []string{inputEvent.EventID()}) {
				t.Errorf("call to /get_missing_events wrong latest events: got %v want %v", missing.LatestEvents, inputEvent.EventID())
			}
			return gomatrixserverlib.RespMissingEvents{
				Events: []*gomatrixserverlib.Event{
					prevEvent.Unwrap(),
				},
			}, nil
		},
	}

	pdus := []json.RawMessage{
		inputEvent.JSON(),
	}
	txn := mustCreateTransaction(rsAPI, cli, pdus)
	mustProcessTransaction(t, txn, nil)
	assertInputRoomEvents(t, rsAPI.inputRoomEvents, []*gomatrixserverlib.HeaderedEvent{prevEvent, inputEvent})
}

// The purpose of this test is to check that when there are missing prev_events and we still haven't been able to fill
// in the hole with /get_missing_events that the state BEFORE the events we want to persist is fetched via /state_ids
// and /event. It works by setting PrevEventsExist=false in the roomserver query response, resulting in
// a call to /get_missing_events which returns 1 out of the 2 events it needs to fill in the gap. Synapse and Dendrite
// both give up after 1x /get_missing_events call, relying on requesting the state AFTER the missing event in order to
// continue. The DAG looks something like:
// FE           GME    TXN
//  A ---> B ---> C ---> D
// TXN=event in the txn, GME=response to /get_missing_events, FE=roomserver's forward extremity. Should result in:
// - /state_ids?event=B is requested, then /event/B to get the state AFTER B. B is a state event.
// - state resolution is done to check C is allowed.
// This results in B being sent as an outlier FIRST, then C,D.
func TestTransactionFetchMissingStateByStateIDs(t *testing.T) {
	eventA := testEvents[len(testEvents)-5]
	// this is also len(testEvents)-4
	eventB := testStateEvents[gomatrixserverlib.StateKeyTuple{
		EventType: gomatrixserverlib.MRoomPowerLevels,
		StateKey:  "",
	}]
	eventC := testEvents[len(testEvents)-3]
	eventD := testEvents[len(testEvents)-2]
	fmt.Println("a:", eventA.EventID())
	fmt.Println("b:", eventB.EventID())
	fmt.Println("c:", eventC.EventID())
	fmt.Println("d:", eventD.EventID())
	var rsAPI *testRoomserverAPI
	rsAPI = &testRoomserverAPI{
		queryStateAfterEvents: func(req *api.QueryStateAfterEventsRequest) api.QueryStateAfterEventsResponse {
			omitTuples := []gomatrixserverlib.StateKeyTuple{
				{
					EventType: gomatrixserverlib.MRoomPowerLevels,
					StateKey:  "",
				},
			}
			askingForEvent := req.PrevEventIDs[0]
			haveEventB := false
			haveEventC := false
			for _, ev := range rsAPI.inputRoomEvents {
				switch ev.Event.EventID() {
				case eventB.EventID():
					haveEventB = true
					omitTuples = nil // include event B now
				case eventC.EventID():
					haveEventC = true
				}
			}
			prevEventExists := false
			if askingForEvent == eventC.EventID() {
				prevEventExists = haveEventC
			} else if askingForEvent == eventB.EventID() {
				prevEventExists = haveEventB
			}
			var stateEvents []*gomatrixserverlib.HeaderedEvent
			if prevEventExists {
				stateEvents = fromStateTuples(req.StateToFetch, omitTuples)
			}
			return api.QueryStateAfterEventsResponse{
				PrevEventsExist: prevEventExists,
				RoomExists:      true,
				StateEvents:     stateEvents,
			}
		},

		queryMissingAuthPrevEvents: func(req *api.QueryMissingAuthPrevEventsRequest) api.QueryMissingAuthPrevEventsResponse {
			askingForEvent := req.PrevEventIDs[0]
			haveEventB := false
			haveEventC := false
			for _, ev := range rsAPI.inputRoomEvents {
				switch ev.Event.EventID() {
				case eventB.EventID():
					haveEventB = true
				case eventC.EventID():
					haveEventC = true
				}
			}
			prevEventExists := false
			if askingForEvent == eventC.EventID() {
				prevEventExists = haveEventC
			} else if askingForEvent == eventB.EventID() {
				prevEventExists = haveEventB
			}

			var missingPrevEvent []string
			if !prevEventExists {
				missingPrevEvent = []string{"test"}
			}

			return api.QueryMissingAuthPrevEventsResponse{
				RoomExists:          true,
				MissingAuthEventIDs: []string{},
				MissingPrevEventIDs: missingPrevEvent,
			}
		},

		queryLatestEventsAndState: func(req *api.QueryLatestEventsAndStateRequest) api.QueryLatestEventsAndStateResponse {
			omitTuples := []gomatrixserverlib.StateKeyTuple{
				{EventType: gomatrixserverlib.MRoomPowerLevels, StateKey: ""},
			}
			return api.QueryLatestEventsAndStateResponse{
				RoomExists: true,
				Depth:      eventA.Depth(),
				LatestEvents: []gomatrixserverlib.EventReference{
					eventA.EventReference(),
				},
				StateEvents: fromStateTuples(req.StateToFetch, omitTuples),
			}
		},
		queryEventsByID: func(req *api.QueryEventsByIDRequest) api.QueryEventsByIDResponse {
			var res api.QueryEventsByIDResponse
			fmt.Println("queryEventsByID ", req.EventIDs)
			for _, wantEventID := range req.EventIDs {
				for _, ev := range testStateEvents {
					// roomserver is missing the power levels event unless it's been sent to us recently as an outlier
					if wantEventID == eventB.EventID() {
						fmt.Println("Asked for pl event")
						for _, inEv := range rsAPI.inputRoomEvents {
							fmt.Println("recv ", inEv.Event.EventID())
							if inEv.Event.EventID() == wantEventID {
								res.Events = append(res.Events, inEv.Event)
								break
							}
						}
						continue
					}
					if ev.EventID() == wantEventID {
						res.Events = append(res.Events, ev)
					}
				}
			}
			return res
		},
	}
	// /state_ids for event B returns every state event but B (it's the state before)
	var authEventIDs []string
	var stateEventIDs []string
	for _, ev := range testStateEvents {
		if ev.EventID() == eventB.EventID() {
			continue
		}
		// state res checks what auth events you give it, and this isn't a valid auth event
		if ev.Type() != gomatrixserverlib.MRoomHistoryVisibility {
			authEventIDs = append(authEventIDs, ev.EventID())
		}
		stateEventIDs = append(stateEventIDs, ev.EventID())
	}
	cli := &txnFedClient{
		stateIDs: map[string]gomatrixserverlib.RespStateIDs{
			eventB.EventID(): {
				StateEventIDs: stateEventIDs,
				AuthEventIDs:  authEventIDs,
			},
		},
		// /event for event B returns it
		getEvent: map[string]gomatrixserverlib.Transaction{
			eventB.EventID(): {
				PDUs: []json.RawMessage{
					eventB.JSON(),
				},
			},
		},
		// /get_missing_events should be done exactly once
		getMissingEvents: func(missing gomatrixserverlib.MissingEvents) (res gomatrixserverlib.RespMissingEvents, err error) {
			if !reflect.DeepEqual(missing.EarliestEvents, []string{eventA.EventID()}) {
				t.Errorf("call to /get_missing_events wrong earliest events: got %v want %v", missing.EarliestEvents, eventA.EventID())
			}
			if !reflect.DeepEqual(missing.LatestEvents, []string{eventD.EventID()}) {
				t.Errorf("call to /get_missing_events wrong latest events: got %v want %v", missing.LatestEvents, eventD.EventID())
			}
			// just return event C, not event B so /state_ids logic kicks in as there will STILL be missing prev_events
			return gomatrixserverlib.RespMissingEvents{
				Events: []*gomatrixserverlib.Event{
					eventC.Unwrap(),
				},
			}, nil
		},
	}

	pdus := []json.RawMessage{
		eventD.JSON(),
	}
	txn := mustCreateTransaction(rsAPI, cli, pdus)
	mustProcessTransaction(t, txn, nil)
	assertInputRoomEvents(t, rsAPI.inputRoomEvents, []*gomatrixserverlib.HeaderedEvent{eventB, eventC, eventD})
}
*/
