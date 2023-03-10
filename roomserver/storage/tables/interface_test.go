package tables

import (
	"testing"

	"github.com/matrix-org/dendrite/test"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/stretchr/testify/assert"
)

func TestExtractContentValue(t *testing.T) {
	alice := test.NewUser(t)
	room := test.NewRoom(t, alice)

	tests := []struct {
		name  string
		event *gomatrixserverlib.HeaderedEvent
		want  string
	}{
		{
			name:  "returns creator ID for create events",
			event: room.Events()[0],
			want:  alice.ID,
		},
		{
			name:  "returns the alias for canonical alias events",
			event: room.CreateEvent(t, alice, gomatrixserverlib.MRoomCanonicalAlias, map[string]string{"alias": "#test:test"}),
			want:  "#test:test",
		},
		{
			name:  "returns the history_visibility for history visibility events",
			event: room.CreateEvent(t, alice, gomatrixserverlib.MRoomHistoryVisibility, map[string]string{"history_visibility": "shared"}),
			want:  "shared",
		},
		{
			name:  "returns the join rules for join_rules events",
			event: room.CreateEvent(t, alice, gomatrixserverlib.MRoomJoinRules, map[string]string{"join_rule": "public"}),
			want:  "public",
		},
		{
			name:  "returns the membership for room_member events",
			event: room.CreateEvent(t, alice, gomatrixserverlib.MRoomMember, map[string]string{"membership": "join"}, test.WithStateKey(alice.ID)),
			want:  "join",
		},
		{
			name:  "returns the room name for room_name events",
			event: room.CreateEvent(t, alice, gomatrixserverlib.MRoomName, map[string]string{"name": "testing"}, test.WithStateKey(alice.ID)),
			want:  "testing",
		},
		{
			name:  "returns the room avatar for avatar events",
			event: room.CreateEvent(t, alice, gomatrixserverlib.MRoomAvatar, map[string]string{"url": "mxc://testing"}, test.WithStateKey(alice.ID)),
			want:  "mxc://testing",
		},
		{
			name:  "returns the room topic for topic events",
			event: room.CreateEvent(t, alice, gomatrixserverlib.MRoomTopic, map[string]string{"topic": "testing"}, test.WithStateKey(alice.ID)),
			want:  "testing",
		},
		{
			name:  "returns guest_access for guest access events",
			event: room.CreateEvent(t, alice, "m.room.guest_access", map[string]string{"guest_access": "forbidden"}, test.WithStateKey(alice.ID)),
			want:  "forbidden",
		},
		{
			name:  "returns empty string if key can't be found or unknown event",
			event: room.CreateEvent(t, alice, "idontexist", nil),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, ExtractContentValue(tt.event), "ExtractContentValue(%v)", tt.event)
		})
	}
}
