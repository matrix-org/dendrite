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
	"testing"

	"github.com/matrix-org/gomatrixserverlib"
)

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
	ce := ToClientEvent(ev, FormatAll)
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
	if ce.Sender != ev.Sender() {
		t.Errorf("ClientEvent.Sender: wanted %s, got %s", ev.Sender(), ce.Sender)
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
	ce := ToClientEvent(ev, FormatSync)
	if ce.RoomID != "" {
		t.Errorf("ClientEvent.RoomID: wanted '', got %s", ce.RoomID)
	}
}
