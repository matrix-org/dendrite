/* Copyright 2024 New Vector Ltd.
 * Copyright 2017 Vector Creations Ltd
 *
 * SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
 * Please see LICENSE files in the repository root for full details.
 */

package synctypes

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
)

func queryUserIDForSender(senderID spec.SenderID) (*spec.UserID, error) {
	if senderID == "" {
		return nil, nil
	}

	return spec.NewUserID(string(senderID), true)
}

const testSenderID = "testSenderID"
const testUserID = "@test:localhost"

type EventFieldsToVerify struct {
	EventID        string
	Type           string
	OriginServerTS spec.Timestamp
	StateKey       *string
	Content        spec.RawJSON
	Unsigned       spec.RawJSON
	Sender         string
	Depth          int64
	PrevEvents     []string
	AuthEvents     []string
}

func verifyEventFields(t *testing.T, got EventFieldsToVerify, want EventFieldsToVerify) {
	if got.EventID != want.EventID {
		t.Errorf("ClientEvent.EventID: wanted %s, got %s", want.EventID, got.EventID)
	}
	if got.OriginServerTS != want.OriginServerTS {
		t.Errorf("ClientEvent.OriginServerTS: wanted %d, got %d", want.OriginServerTS, got.OriginServerTS)
	}
	if got.StateKey == nil && want.StateKey != nil {
		t.Errorf("ClientEvent.StateKey: no state key present when one was wanted: %s", *want.StateKey)
	}
	if got.StateKey != nil && want.StateKey == nil {
		t.Errorf("ClientEvent.StateKey: state key present when one was not wanted: %s", *got.StateKey)
	}
	if got.StateKey != nil && want.StateKey != nil && *got.StateKey != *want.StateKey {
		t.Errorf("ClientEvent.StateKey: wanted %s, got %s", *want.StateKey, *got.StateKey)
	}
	if got.Type != want.Type {
		t.Errorf("ClientEvent.Type: wanted %s, got %s", want.Type, got.Type)
	}
	if !bytes.Equal(got.Content, want.Content) {
		t.Errorf("ClientEvent.Content: wanted %s, got %s", string(want.Content), string(got.Content))
	}
	if !bytes.Equal(got.Unsigned, want.Unsigned) {
		t.Errorf("ClientEvent.Unsigned: wanted %s, got %s", string(want.Unsigned), string(got.Unsigned))
	}
	if got.Sender != want.Sender {
		t.Errorf("ClientEvent.Sender: wanted %s, got %s", want.Sender, got.Sender)
	}
	if got.Depth != want.Depth {
		t.Errorf("ClientEvent.Depth: wanted %d, got %d", want.Depth, got.Depth)
	}
	if !reflect.DeepEqual(got.PrevEvents, want.PrevEvents) {
		t.Errorf("ClientEvent.PrevEvents: wanted %v, got %v", want.PrevEvents, got.PrevEvents)
	}
	if !reflect.DeepEqual(got.AuthEvents, want.AuthEvents) {
		t.Errorf("ClientEvent.AuthEvents: wanted %v, got %v", want.AuthEvents, got.AuthEvents)
	}
}

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
	ce, err := ToClientEvent(ev, FormatAll, func(roomID spec.RoomID, senderID spec.SenderID) (*spec.UserID, error) {
		return queryUserIDForSender(senderID)
	})
	if err != nil {
		t.Fatalf("failed to create ClientEvent: %s", err)
	}

	verifyEventFields(t,
		EventFieldsToVerify{
			EventID:        ce.EventID,
			Type:           ce.Type,
			OriginServerTS: ce.OriginServerTS,
			StateKey:       ce.StateKey,
			Content:        ce.Content,
			Unsigned:       ce.Unsigned,
			Sender:         ce.Sender,
		},
		EventFieldsToVerify{
			EventID:        ev.EventID(),
			Type:           ev.Type(),
			OriginServerTS: ev.OriginServerTS(),
			StateKey:       &sk,
			Content:        ev.Content(),
			Unsigned:       ev.Unsigned(),
			Sender:         userID.String(),
		})

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
	ce, err := ToClientEvent(ev, FormatSync, func(roomID spec.RoomID, senderID spec.SenderID) (*spec.UserID, error) {
		return queryUserIDForSender(senderID)
	})
	if err != nil {
		t.Fatalf("failed to create ClientEvent: %s", err)
	}
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
	ce, err := ToClientEvent(ev, FormatSyncFederation, func(roomID spec.RoomID, senderID spec.SenderID) (*spec.UserID, error) {
		return queryUserIDForSender(senderID)
	})
	if err != nil {
		t.Fatalf("failed to create ClientEvent: %s", err)
	}

	verifyEventFields(t,
		EventFieldsToVerify{
			EventID:        ce.EventID,
			Type:           ce.Type,
			OriginServerTS: ce.OriginServerTS,
			StateKey:       ce.StateKey,
			Content:        ce.Content,
			Unsigned:       ce.Unsigned,
			Sender:         ce.Sender,
			Depth:          ce.Depth,
			PrevEvents:     ce.PrevEvents,
			AuthEvents:     ce.AuthEvents,
		},
		EventFieldsToVerify{
			EventID:        ev.EventID(),
			Type:           ev.Type(),
			OriginServerTS: ev.OriginServerTS(),
			StateKey:       &sk,
			Content:        ev.Content(),
			Unsigned:       ev.Unsigned(),
			Sender:         userID.String(),
			Depth:          ev.Depth(),
			PrevEvents:     ev.PrevEventIDs(),
			AuthEvents:     ev.AuthEventIDs(),
		})
}

func userIDForSender(roomID spec.RoomID, senderID spec.SenderID) (*spec.UserID, error) {
	if senderID == "unknownSenderID" {
		return nil, fmt.Errorf("Cannot find userID")
	}
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
	sk := testSenderID
	verifyEventFields(t,
		EventFieldsToVerify{
			EventID:        ce.EventID,
			Type:           ce.Type,
			OriginServerTS: ce.OriginServerTS,
			StateKey:       ce.StateKey,
			Content:        ce.Content,
			Unsigned:       ce.Unsigned,
			Sender:         ce.Sender,
			Depth:          ce.Depth,
			PrevEvents:     ce.PrevEvents,
			AuthEvents:     ce.AuthEvents,
		},
		EventFieldsToVerify{
			EventID:        ev.EventID(),
			Type:           ev.Type(),
			OriginServerTS: ev.OriginServerTS(),
			StateKey:       &sk,
			Content:        ev.Content(),
			Unsigned:       ev.Unsigned(),
			Sender:         testSenderID,
			Depth:          ev.Depth(),
			PrevEvents:     ev.PrevEventIDs(),
			AuthEvents:     ev.AuthEventIDs(),
		})

	ce2 := clientEvents[1]
	verifyEventFields(t,
		EventFieldsToVerify{
			EventID:        ce2.EventID,
			Type:           ce2.Type,
			OriginServerTS: ce2.OriginServerTS,
			StateKey:       ce2.StateKey,
			Content:        ce2.Content,
			Unsigned:       ce2.Unsigned,
			Sender:         ce2.Sender,
			Depth:          ce2.Depth,
			PrevEvents:     ce2.PrevEvents,
			AuthEvents:     ce2.AuthEvents,
		},
		EventFieldsToVerify{
			EventID:        ev2.EventID(),
			Type:           ev2.Type(),
			OriginServerTS: ev2.OriginServerTS(),
			StateKey:       &sk,
			Content:        ev2.Content(),
			Unsigned:       ev2.Unsigned(),
			Sender:         testSenderID,
			Depth:          ev2.Depth(),
			PrevEvents:     ev2.PrevEventIDs(),
			AuthEvents:     ev2.AuthEventIDs(),
		})
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
	sk := testUserID
	verifyEventFields(t,
		EventFieldsToVerify{
			EventID:        ce.EventID,
			Type:           ce.Type,
			OriginServerTS: ce.OriginServerTS,
			StateKey:       ce.StateKey,
			Content:        ce.Content,
			Unsigned:       ce.Unsigned,
			Sender:         ce.Sender,
		},
		EventFieldsToVerify{
			EventID:        ev.EventID(),
			Type:           ev.Type(),
			OriginServerTS: ev.OriginServerTS(),
			StateKey:       &sk,
			Content:        ev.Content(),
			Unsigned:       ev.Unsigned(),
			Sender:         testUserID,
		})

	var prev PrevEventRef
	prev.PrevContent = []byte(`{"name": "Goodbye World 2"}`)
	prev.PrevSenderID = testUserID
	expectedUnsigned, _ := json.Marshal(prev)

	ce2 := clientEvents[1]
	verifyEventFields(t,
		EventFieldsToVerify{
			EventID:        ce2.EventID,
			Type:           ce2.Type,
			OriginServerTS: ce2.OriginServerTS,
			StateKey:       ce2.StateKey,
			Content:        ce2.Content,
			Unsigned:       ce2.Unsigned,
			Sender:         ce2.Sender,
		},
		EventFieldsToVerify{
			EventID:        ev2.EventID(),
			Type:           ev2.Type(),
			OriginServerTS: ev2.OriginServerTS(),
			StateKey:       &sk,
			Content:        ev2.Content(),
			Unsigned:       expectedUnsigned,
			Sender:         testUserID,
		})
}

func TestToClientEventsFormatSyncUnknownPrevSender(t *testing.T) { // nolint: gocyclo
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
            "prev_sender": "unknownSenderID"
		},
        "depth": 9	
    }`), false)
	if err != nil {
		t.Fatalf("failed to create Event: %s", err)
	}

	clientEvents := ToClientEvents([]gomatrixserverlib.PDU{ev, ev2}, FormatSync, userIDForSender)
	ce := clientEvents[0]
	sk := testUserID
	verifyEventFields(t,
		EventFieldsToVerify{
			EventID:        ce.EventID,
			Type:           ce.Type,
			OriginServerTS: ce.OriginServerTS,
			StateKey:       ce.StateKey,
			Content:        ce.Content,
			Unsigned:       ce.Unsigned,
			Sender:         ce.Sender,
		},
		EventFieldsToVerify{
			EventID:        ev.EventID(),
			Type:           ev.Type(),
			OriginServerTS: ev.OriginServerTS(),
			StateKey:       &sk,
			Content:        ev.Content(),
			Unsigned:       ev.Unsigned(),
			Sender:         testUserID,
		})

	var prev PrevEventRef
	prev.PrevContent = []byte(`{"name": "Goodbye World 2"}`)
	prev.PrevSenderID = "unknownSenderID"
	expectedUnsigned, _ := json.Marshal(prev)

	ce2 := clientEvents[1]
	verifyEventFields(t,
		EventFieldsToVerify{
			EventID:        ce2.EventID,
			Type:           ce2.Type,
			OriginServerTS: ce2.OriginServerTS,
			StateKey:       ce2.StateKey,
			Content:        ce2.Content,
			Unsigned:       ce2.Unsigned,
			Sender:         ce2.Sender,
		},
		EventFieldsToVerify{
			EventID:        ev2.EventID(),
			Type:           ev2.Type(),
			OriginServerTS: ev2.OriginServerTS(),
			StateKey:       &sk,
			Content:        ev2.Content(),
			Unsigned:       expectedUnsigned,
			Sender:         testUserID,
		})
}
