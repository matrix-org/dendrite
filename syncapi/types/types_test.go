package types

import (
	"encoding/json"
	"testing"

	"github.com/matrix-org/gomatrixserverlib"
)

func TestSyncTokens(t *testing.T) {
	shouldPass := map[string]string{
		"s4_0_0_0_0_0_0_0_3": StreamingToken{4, 0, 0, 0, 0, 0, 0, 0, 3}.String(),
		"s3_1_0_0_0_0_2_0_5": StreamingToken{3, 1, 0, 0, 0, 0, 2, 0, 5}.String(),
		"s3_1_2_3_5_0_0_0_6": StreamingToken{3, 1, 2, 3, 5, 0, 0, 0, 6}.String(),
		"t3_1":               TopologyToken{3, 1}.String(),
	}

	for a, b := range shouldPass {
		if a != b {
			t.Errorf("expected %q, got %q", a, b)
		}
	}

	shouldFail := []string{
		"",
		"s_",
		"a3_4",
		"b",
		"b-1",
		"-4",
		"2",
	}

	for _, f := range append(shouldFail, "t1_2") {
		if _, err := NewStreamTokenFromString(f); err == nil {
			t.Errorf("NewStreamTokenFromString %q should have failed", f)
		}
	}

	for _, f := range append(shouldFail, "s1_2_3_4") {
		if _, err := NewTopologyTokenFromString(f); err == nil {
			t.Errorf("NewTopologyTokenFromString %q should have failed", f)
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
