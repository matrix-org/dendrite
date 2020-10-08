package types

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/matrix-org/gomatrixserverlib"
)

func TestNewSyncTokenWithLogs(t *testing.T) {
	tests := map[string]*StreamingToken{
		"s4_0": &StreamingToken{
			syncToken: syncToken{Type: "s", Positions: []StreamPosition{4, 0}},
			logs:      make(map[string]*LogPosition),
		},
		"s4_0.dl-0-123": &StreamingToken{
			syncToken: syncToken{Type: "s", Positions: []StreamPosition{4, 0}},
			logs: map[string]*LogPosition{
				"dl": &LogPosition{
					Partition: 0,
					Offset:    123,
				},
			},
		},
		"s4_0.ab-1-14419482332.dl-0-123": &StreamingToken{
			syncToken: syncToken{Type: "s", Positions: []StreamPosition{4, 0}},
			logs: map[string]*LogPosition{
				"ab": &LogPosition{
					Partition: 1,
					Offset:    14419482332,
				},
				"dl": &LogPosition{
					Partition: 0,
					Offset:    123,
				},
			},
		},
	}
	for tok, want := range tests {
		got, err := NewStreamTokenFromString(tok)
		if err != nil {
			if want == nil {
				continue // error expected
			}
			t.Errorf("%s errored: %s", tok, err)
			continue
		}
		if !reflect.DeepEqual(got, *want) {
			t.Errorf("%s mismatch: got %v want %v", tok, got, want)
		}
		gotStr := got.String()
		if gotStr != tok {
			t.Errorf("%s reserialisation mismatch: got %s want %s", tok, gotStr, tok)
		}
	}
}

func TestNewSyncTokenFromString(t *testing.T) {
	shouldPass := map[string]syncToken{
		"s4_0": NewStreamToken(4, 0, nil).syncToken,
		"s3_1": NewStreamToken(3, 1, nil).syncToken,
		"t3_1": NewTopologyToken(3, 1).syncToken,
	}

	shouldFail := []string{
		"",
		"s_1",
		"s_",
		"a3_4",
		"b",
		"b-1",
		"-4",
		"2",
	}

	for test, expected := range shouldPass {
		result, _, err := newSyncTokenFromString(test)
		if err != nil {
			t.Error(err)
		}
		if result.String() != expected.String() {
			t.Errorf("%s expected %v but got %v", test, expected.String(), result.String())
		}
	}

	for _, test := range shouldFail {
		if _, _, err := newSyncTokenFromString(test); err == nil {
			t.Errorf("input '%v' should have errored but didn't", test)
		}
	}
}

func TestNewInviteResponse(t *testing.T) {
	event := `{"auth_events":["$SbSsh09j26UAXnjd3RZqf2lyA3Kw2sY_VZJVZQAV9yA","$EwL53onrLwQ5gL8Dv3VrOOCvHiueXu2ovLdzqkNi3lo","$l2wGmz9iAwevBDGpHT_xXLUA5O8BhORxWIGU1cGi1ZM","$GsWFJLXgdlF5HpZeyWkP72tzXYWW3uQ9X28HBuTztHE"],"content":{"avatar_url":"","displayname":"neilalexander","membership":"invite"},"depth":9,"hashes":{"sha256":"8p+Ur4f8vLFX6mkIXhxI0kegPG7X3tWy56QmvBkExAg"},"origin":"matrix.org","origin_server_ts":1602087113066,"prev_events":["$1v-O6tNwhOZcA8bvCYY-Dnj1V2ZDE58lLPxtlV97S28"],"prev_state":[],"room_id":"!XbeXirGWSPXbEaGokF:matrix.org","sender":"@neilalexander:matrix.org","signatures":{"dendrite.neilalexander.dev":{"ed25519:BMJi":"05KQ5lPw0cSFsE4A0x1z7vi/3cc8bG4WHUsFWYkhxvk/XkXMGIYAYkpNThIvSeLfdcHlbm/k10AsBSKH8Uq4DA"},"matrix.org":{"ed25519:a_RXGa":"jeovuHr9E/x0sHbFkdfxDDYV/EyoeLi98douZYqZ02iYddtKhfB7R3WLay/a+D3V3V7IW0FUmPh/A404x5sYCw"}},"state_key":"@neilalexander:dendrite.neilalexander.dev","type":"m.room.member","unsigned":{"age":2512,"invite_room_state":[{"content":{"join_rule":"invite"},"sender":"@neilalexander:matrix.org","state_key":"","type":"m.room.join_rules"},{"content":{"avatar_url":"mxc://matrix.org/BpDaozLwgLnlNStxDxvLzhPr","displayname":"neilalexander","membership":"join"},"sender":"@neilalexander:matrix.org","state_key":"@neilalexander:matrix.org","type":"m.room.member"},{"content":{"name":"Test room"},"sender":"@neilalexander:matrix.org","state_key":"","type":"m.room.name"}]},"_room_version":"5"}`
	expected := `{"invite_state":{"events":[{"content":{"join_rule":"invite"},"sender":"@neilalexander:matrix.org","state_key":"","type":"m.room.join_rules"},{"content":{"avatar_url":"mxc://matrix.org/BpDaozLwgLnlNStxDxvLzhPr","displayname":"neilalexander","membership":"join"},"sender":"@neilalexander:matrix.org","state_key":"@neilalexander:matrix.org","type":"m.room.member"},{"content":{"name":"Test room"},"sender":"@neilalexander:matrix.org","state_key":"","type":"m.room.name"},{"content":{"avatar_url":"","displayname":"neilalexander","membership":"invite"},"event_id":"$GQmw8e8-26CQv1QuFoHBHpKF1hQj61Flg3kvv_v_XWs","origin_server_ts":1602087113066,"sender":"@neilalexander:matrix.org","state_key":"@neilalexander:dendrite.neilalexander.dev","type":"m.room.member"}]}}`

	ev, err := gomatrixserverlib.NewEventFromTrustedJSON([]byte(event), false, gomatrixserverlib.RoomVersionV5)
	if err != nil {
		t.Fatal(err)
	}

	res := NewInviteResponse(ev.Headered(gomatrixserverlib.RoomVersionV5))
	j, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}

	if string(j) != expected {
		t.Fatalf("Invite response didn't contain correct info")
	}
}
