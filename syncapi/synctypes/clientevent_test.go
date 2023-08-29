/* Copyright 2017 Vector Creations Ltd
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package synctypes

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
)

const testSenderID = "testSenderID"
const testUserID = "@test:localhost"

func TestToClientEvent(t *testing.T) { // nolint: gocyclo
	ev, err := gomatrixserverlib.MustGetRoomVersion(gomatrixserverlib.RoomVersionV1).NewEventFromTrustedJSON([]byte(`{
		"type": "m.room.name",
		"state_key": "",
		"event_id": "$test:localhost",
		"room_id": "!test:localhost",
		"sender": "@test:localhost",
		"content": {
			"name": "Hello World"
		},
		"origin_server_ts": 123456,
		"unsigned": {
			"prev_content": {
				"name": "Goodbye World"
			}
		}
	}`), false)
	if err != nil {
		t.Fatalf("failed to create Event: %s", err)
	}
	userID, err := spec.NewUserID("@test:localhost", true)
	if err != nil {
		t.Fatalf("failed to create userID: %s", err)
	}
	sk := ""
	ce := ToClientEvent(ev, FormatAll, userID.String(), &sk, ev.Unsigned())
	if ce.EventID != ev.EventID() {
		t.Errorf("ClientEvent.EventID: wanted %s, got %s", ev.EventID(), ce.EventID)
	}
	if ce.OriginServerTS != ev.OriginServerTS() {
		t.Errorf("ClientEvent.OriginServerTS: wanted %d, got %d", ev.OriginServerTS(), ce.OriginServerTS)
	}
	if ce.StateKey == nil || *ce.StateKey != "" {
		t.Errorf("ClientEvent.StateKey: wanted '', got %v", ce.StateKey)
	}
	if ce.Type != ev.Type() {
		t.Errorf("ClientEvent.Type: wanted %s, got %s", ev.Type(), ce.Type)
	}
	if !bytes.Equal(ce.Content, ev.Content()) {
		t.Errorf("ClientEvent.Content: wanted %s, got %s", string(ev.Content()), string(ce.Content))
	}
	if !bytes.Equal(ce.Unsigned, ev.Unsigned()) {
		t.Errorf("ClientEvent.Unsigned: wanted %s, got %s", string(ev.Unsigned()), string(ce.Unsigned))
	}
	if ce.Sender != userID.String() {
		t.Errorf("ClientEvent.Sender: wanted %s, got %s", userID.String(), ce.Sender)
	}
	j, err := json.Marshal(ce)
	if err != nil {
		t.Fatalf("failed to Marshal ClientEvent: %s", err)
	}
	// Marshal sorts keys in structs by the order they are defined in the struct, which is alphabetical
	out := `{"content":{"name":"Hello World"},"event_id":"$test:localhost","origin_server_ts":123456,` +
		`"room_id":"!test:localhost","sender":"@test:localhost","state_key":"","type":"m.room.name",` +
		`"unsigned":{"prev_content":{"name":"Goodbye World"}}}`
	if !bytes.Equal([]byte(out), j) {
		t.Errorf("ClientEvent marshalled to wrong bytes: wanted %s, got %s", out, string(j))
	}
}

func TestToClientFormatSync(t *testing.T) {
	ev, err := gomatrixserverlib.MustGetRoomVersion(gomatrixserverlib.RoomVersionV1).NewEventFromTrustedJSON([]byte(`{
		"type": "m.room.name",
		"state_key": "",
		"event_id": "$test:localhost",
		"room_id": "!test:localhost",
		"sender": "@test:localhost",
		"content": {
			"name": "Hello World"
		},
		"origin_server_ts": 123456,
		"unsigned": {
			"prev_content": {
				"name": "Goodbye World"
			}
		}
	}`), false)
	if err != nil {
		t.Fatalf("failed to create Event: %s", err)
	}
	userID, err := spec.NewUserID("@test:localhost", true)
	if err != nil {
		t.Fatalf("failed to create userID: %s", err)
	}
	sk := ""
	ce := ToClientEvent(ev, FormatSync, userID.String(), &sk, ev.Unsigned())
	if ce.RoomID != "" {
		t.Errorf("ClientEvent.RoomID: wanted '', got %s", ce.RoomID)
	}
}

func TestToClientEventFormatSyncFederation(t *testing.T) { // nolint: gocyclo
	ev, err := gomatrixserverlib.MustGetRoomVersion(gomatrixserverlib.RoomVersionV10).NewEventFromTrustedJSON([]byte(`{
		"type": "m.room.name",
		"state_key": "",
		"event_id": "$test:localhost",
		"room_id": "!test:localhost",
		"sender": "@test:localhost",
		"content": {
			"name": "Hello World"
		},
		"origin_server_ts": 123456,
		"unsigned": {
			"prev_content": {
				"name": "Goodbye World"
			}
		},
        "depth": 8,
        "prev_events": [
          "$f597Tp0Mm1PPxEgiprzJc2cZAjVhxCxACOGuwJb33Oo"
        ],
        "auth_events": [
          "$Bj0ZGgX6VTqAQdqKH4ZG3l6rlbxY3rZlC5D3MeuK1OQ",
          "$QsMs6A1PUVUhgSvmHBfpqEYJPgv4DXt96r8P2AK7iXQ",
          "$tBteKtlnFiwlmPJsv0wkKTMEuUVWpQH89H7Xskxve1Q"
        ]
	}`), false)
	if err != nil {
		t.Fatalf("failed to create Event: %s", err)
	}
	userID, err := spec.NewUserID("@test:localhost", true)
	if err != nil {
		t.Fatalf("failed to create userID: %s", err)
	}
	sk := ""
	ce := ToClientEvent(ev, FormatSyncFederation, userID.String(), &sk, ev.Unsigned())
	if ce.EventID != ev.EventID() {
		t.Errorf("ClientEvent.EventID: wanted %s, got %s", ev.EventID(), ce.EventID)
	}
	if ce.OriginServerTS != ev.OriginServerTS() {
		t.Errorf("ClientEvent.OriginServerTS: wanted %d, got %d", ev.OriginServerTS(), ce.OriginServerTS)
	}
	if ce.StateKey == nil || *ce.StateKey != "" {
		t.Errorf("ClientEvent.StateKey: wanted '', got %v", ce.StateKey)
	}
	if ce.Type != ev.Type() {
		t.Errorf("ClientEvent.Type: wanted %s, got %s", ev.Type(), ce.Type)
	}
	if !bytes.Equal(ce.Content, ev.Content()) {
		t.Errorf("ClientEvent.Content: wanted %s, got %s", string(ev.Content()), string(ce.Content))
	}
	if !bytes.Equal(ce.Unsigned, ev.Unsigned()) {
		t.Errorf("ClientEvent.Unsigned: wanted %s, got %s", string(ev.Unsigned()), string(ce.Unsigned))
	}
	if ce.Sender != userID.String() {
		t.Errorf("ClientEvent.Sender: wanted %s, got %s", userID.String(), ce.Sender)
	}
	if ce.Depth != ev.Depth() {
		t.Errorf("ClientEvent.Depth: wanted %d, got %d", ev.Depth(), ce.Depth)
	}
	if !reflect.DeepEqual(ce.PrevEvents, ev.PrevEventIDs()) {
		t.Errorf("ClientEvent.PrevEvents: wanted %v, got %v", ev.PrevEventIDs(), ce.PrevEvents)
	}
	if !reflect.DeepEqual(ce.AuthEvents, ev.AuthEventIDs()) {
		t.Errorf("ClientEvent.AuthEvents: wanted %v, got %v", ev.AuthEventIDs(), ce.AuthEvents)
	}
}

func userIDForSender(roomID spec.RoomID, senderID spec.SenderID) (*spec.UserID, error) {
	return spec.NewUserID(testUserID, true)
}

func TestToClientEventsFormatSyncFederation(t *testing.T) { // nolint: gocyclo
	ev, err := gomatrixserverlib.MustGetRoomVersion(gomatrixserverlib.RoomVersionPseudoIDs).NewEventFromTrustedJSON([]byte(`{
		"type": "m.room.name",
        "state_key": "testSenderID",
		"event_id": "$test:localhost",
		"room_id": "!test:localhost",
		"sender": "testSenderID",
		"content": {
			"name": "Hello World"
		},
		"origin_server_ts": 123456,
		"unsigned": {
			"prev_content": {
				"name": "Goodbye World"
			}
		},
        "depth": 8,
        "prev_events": [
          "$f597Tp0Mm1PPxEgiprzJc2cZAjVhxCxACOGuwJb33Oo"
        ],
        "auth_events": [
          "$Bj0ZGgX6VTqAQdqKH4ZG3l6rlbxY3rZlC5D3MeuK1OQ",
          "$QsMs6A1PUVUhgSvmHBfpqEYJPgv4DXt96r8P2AK7iXQ",
          "$tBteKtlnFiwlmPJsv0wkKTMEuUVWpQH89H7Xskxve1Q"
        ]
	}`), false)
	if err != nil {
		t.Fatalf("failed to create Event: %s", err)
	}
	ev2, err := gomatrixserverlib.MustGetRoomVersion(gomatrixserverlib.RoomVersionPseudoIDs).NewEventFromTrustedJSON([]byte(`{
		"type": "m.room.name",
        "state_key": "testSenderID",
		"event_id": "$test2:localhost",
		"room_id": "!test:localhost",
		"sender": "testSenderID",
		"content": {
			"name": "Hello World 2"
		},
		"origin_server_ts": 1234567,
		"unsigned": {
			"prev_content": {
				"name": "Goodbye World 2"
			},
            "prev_sender": "testSenderID"
		},
        "depth": 9,
        "prev_events": [
          "$f597Tp0Mm1PPxEgiprzJc2cZAjVhxCxACOGuwJb33Oo"
        ],
        "auth_events": [
          "$Bj0ZGgX6VTqAQdqKH4ZG3l6rlbxY3rZlC5D3MeuK1OQ",
          "$QsMs6A1PUVUhgSvmHBfpqEYJPgv4DXt96r8P2AK7iXQ",
          "$tBteKtlnFiwlmPJsv0wkKTMEuUVWpQH89H7Xskxve1Q"
        ]
    }`), false)
	if err != nil {
		t.Fatalf("failed to create Event: %s", err)
	}

	clientEvents := ToClientEvents([]gomatrixserverlib.PDU{ev, ev2}, FormatSyncFederation, userIDForSender)
	ce := clientEvents[0]
	if ce.EventID != ev.EventID() {
		t.Errorf("ClientEvent.EventID: wanted %s, got %s", ev.EventID(), ce.EventID)
	}
	if ce.OriginServerTS != ev.OriginServerTS() {
		t.Errorf("ClientEvent.OriginServerTS: wanted %d, got %d", ev.OriginServerTS(), ce.OriginServerTS)
	}
	if ce.StateKey == nil || *ce.StateKey != testSenderID {
		t.Errorf("ClientEvent.StateKey: wanted %s, got %v", testSenderID, ce.StateKey)
	}
	if ce.Type != ev.Type() {
		t.Errorf("ClientEvent.Type: wanted %s, got %s", ev.Type(), ce.Type)
	}
	if !bytes.Equal(ce.Content, ev.Content()) {
		t.Errorf("ClientEvent.Content: wanted %s, got %s", string(ev.Content()), string(ce.Content))
	}
	if !bytes.Equal(ce.Unsigned, ev.Unsigned()) {
		t.Errorf("ClientEvent.Unsigned: wanted %s, got %s", string(ev.Unsigned()), string(ce.Unsigned))
	}
	if ce.Sender != testSenderID {
		t.Errorf("ClientEvent.Sender: wanted %s, got %s", testSenderID, ce.Sender)
	}
	if ce.Depth != ev.Depth() {
		t.Errorf("ClientEvent.Depth: wanted %d, got %d", ev.Depth(), ce.Depth)
	}
	if !reflect.DeepEqual(ce.PrevEvents, ev.PrevEventIDs()) {
		t.Errorf("ClientEvent.PrevEvents: wanted %v, got %v", ev.PrevEventIDs(), ce.PrevEvents)
	}
	if !reflect.DeepEqual(ce.AuthEvents, ev.AuthEventIDs()) {
		t.Errorf("ClientEvent.AuthEvents: wanted %v, got %v", ev.AuthEventIDs(), ce.AuthEvents)
	}

	ce2 := clientEvents[1]
	if ce2.EventID != ev2.EventID() {
		t.Errorf("ClientEvent.EventID: wanted %s, got %s", ev2.EventID(), ce2.EventID)
	}
	if ce2.OriginServerTS != ev2.OriginServerTS() {
		t.Errorf("ClientEvent.OriginServerTS: wanted %d, got %d", ev2.OriginServerTS(), ce2.OriginServerTS)
	}
	if ce2.StateKey == nil || *ce.StateKey != testSenderID {
		t.Errorf("ClientEvent.StateKey: wanted %s, got %v", testSenderID, ce2.StateKey)
	}
	if ce2.Type != ev2.Type() {
		t.Errorf("ClientEvent.Type: wanted %s, got %s", ev2.Type(), ce2.Type)
	}
	if !bytes.Equal(ce2.Content, ev2.Content()) {
		t.Errorf("ClientEvent.Content: wanted %s, got %s", string(ev2.Content()), string(ce2.Content))
	}
	if !bytes.Equal(ce2.Unsigned, ev2.Unsigned()) {
		t.Errorf("ClientEvent.Unsigned: wanted %s, got %s", string(ev2.Unsigned()), string(ce2.Unsigned))
	}
	if ce2.Sender != testSenderID {
		t.Errorf("ClientEvent.Sender: wanted %s, got %s", testSenderID, ce2.Sender)
	}
	if ce2.Depth != ev2.Depth() {
		t.Errorf("ClientEvent.Depth: wanted %d, got %d", ev2.Depth(), ce2.Depth)
	}
	if !reflect.DeepEqual(ce2.PrevEvents, ev2.PrevEventIDs()) {
		t.Errorf("ClientEvent.PrevEvents: wanted %v, got %v", ev2.PrevEventIDs(), ce2.PrevEvents)
	}
	if !reflect.DeepEqual(ce2.AuthEvents, ev2.AuthEventIDs()) {
		t.Errorf("ClientEvent.AuthEvents: wanted %v, got %v", ev2.AuthEventIDs(), ce2.AuthEvents)
	}
}

func TestToClientEventsFormatSync(t *testing.T) { // nolint: gocyclo
	ev, err := gomatrixserverlib.MustGetRoomVersion(gomatrixserverlib.RoomVersionPseudoIDs).NewEventFromTrustedJSON([]byte(`{
		"type": "m.room.name",
        "state_key": "testSenderID",
		"event_id": "$test:localhost",
		"room_id": "!test:localhost",
		"sender": "testSenderID",
		"content": {
			"name": "Hello World"
		},
		"origin_server_ts": 123456,
		"unsigned": {
			"prev_content": {
				"name": "Goodbye World"
			}
		}
    }`), false)
	if err != nil {
		t.Fatalf("failed to create Event: %s", err)
	}
	ev2, err := gomatrixserverlib.MustGetRoomVersion(gomatrixserverlib.RoomVersionPseudoIDs).NewEventFromTrustedJSON([]byte(`{
		"type": "m.room.name",
        "state_key": "testSenderID",
		"event_id": "$test2:localhost",
		"room_id": "!test:localhost",
		"sender": "testSenderID",
		"content": {
			"name": "Hello World 2"
		},
		"origin_server_ts": 1234567,
		"unsigned": {
			"prev_content": {
				"name": "Goodbye World 2"
			},
            "prev_sender": "testSenderID"
		},
        "depth": 9	
    }`), false)
	if err != nil {
		t.Fatalf("failed to create Event: %s", err)
	}

	clientEvents := ToClientEvents([]gomatrixserverlib.PDU{ev, ev2}, FormatSync, userIDForSender)
	ce := clientEvents[0]
	if ce.EventID != ev.EventID() {
		t.Errorf("ClientEvent.EventID: wanted %s, got %s", ev.EventID(), ce.EventID)
	}
	if ce.OriginServerTS != ev.OriginServerTS() {
		t.Errorf("ClientEvent.OriginServerTS: wanted %d, got %d", ev.OriginServerTS(), ce.OriginServerTS)
	}
	if ce.StateKey == nil || *ce.StateKey != testUserID {
		t.Errorf("ClientEvent.StateKey: wanted %s, got %v", testUserID, ce.StateKey)
	}
	if ce.Type != ev.Type() {
		t.Errorf("ClientEvent.Type: wanted %s, got %s", ev.Type(), ce.Type)
	}
	if !bytes.Equal(ce.Content, ev.Content()) {
		t.Errorf("ClientEvent.Content: wanted %s, got %s", string(ev.Content()), string(ce.Content))
	}
	if !bytes.Equal(ce.Unsigned, ev.Unsigned()) {
		t.Errorf("ClientEvent.Unsigned: wanted %s, got %s", string(ev.Unsigned()), string(ce.Unsigned))
	}
	if ce.Sender != testUserID {
		t.Errorf("ClientEvent.Sender: wanted %s, got %s", testUserID, ce.Sender)
	}

	var prev PrevEventRef
	prev.PrevContent = []byte(`{"name": "Goodbye World 2"}`)
	prev.PrevSenderID = testUserID
	expectedUnsigned, _ := json.Marshal(prev)

	ce2 := clientEvents[1]
	if ce2.EventID != ev2.EventID() {
		t.Errorf("ClientEvent.EventID: wanted %s, got %s", ev2.EventID(), ce2.EventID)
	}
	if ce2.OriginServerTS != ev2.OriginServerTS() {
		t.Errorf("ClientEvent.OriginServerTS: wanted %d, got %d", ev2.OriginServerTS(), ce2.OriginServerTS)
	}
	if ce2.StateKey == nil || *ce.StateKey != testUserID {
		t.Errorf("ClientEvent.StateKey: wanted %s, got %v", testUserID, ce2.StateKey)
	}
	if ce2.Type != ev2.Type() {
		t.Errorf("ClientEvent.Type: wanted %s, got %s", ev2.Type(), ce2.Type)
	}
	if !bytes.Equal(ce2.Content, ev2.Content()) {
		t.Errorf("ClientEvent.Content: wanted %s, got %s", string(ev2.Content()), string(ce2.Content))
	}
	if !bytes.Equal(ce2.Unsigned, expectedUnsigned) {
		t.Errorf("ClientEvent.Unsigned: wanted %s, got %s", string(expectedUnsigned), string(ce2.Unsigned))
	}
	if ce2.Sender != testUserID {
		t.Errorf("ClientEvent.Sender: wanted %s, got %s", testUserID, ce2.Sender)
	}
}
