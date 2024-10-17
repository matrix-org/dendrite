package auth

import (
	"context"
	"testing"

	"github.com/element-hq/dendrite/roomserver/api"
	"github.com/element-hq/dendrite/test"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
)

type FakeQuerier struct {
	api.QuerySenderIDAPI
}

func (f *FakeQuerier) QueryUserIDForSender(ctx context.Context, roomID spec.RoomID, senderID spec.SenderID) (*spec.UserID, error) {
	return spec.NewUserID(string(senderID), true)
}

func TestIsServerAllowed(t *testing.T) {
	alice := test.NewUser(t)

	tests := []struct {
		name                  string
		want                  bool
		roomFunc              func() *test.Room
		serverName            spec.ServerName
		serverCurrentlyInRoom bool
	}{
		{
			name:     "no servername specified",
			roomFunc: func() *test.Room { return test.NewRoom(t, alice) },
		},
		{
			name:       "no authEvents specified",
			serverName: "test",
			roomFunc:   func() *test.Room { return &test.Room{} },
		},
		{
			name:       "default denied",
			serverName: "test2",
			roomFunc:   func() *test.Room { return test.NewRoom(t, alice) },
		},
		{
			name:       "world readable room",
			serverName: "test",
			roomFunc: func() *test.Room {
				return test.NewRoom(t, alice, test.RoomHistoryVisibility(gomatrixserverlib.HistoryVisibilityWorldReadable))
			},
			want: true,
		},
		{
			name:       "allowed due to alice being joined",
			serverName: "test",
			roomFunc:   func() *test.Room { return test.NewRoom(t, alice) },
			want:       true,
		},
		{
			name:                  "allowed due to 'serverCurrentlyInRoom'",
			serverName:            "test2",
			roomFunc:              func() *test.Room { return test.NewRoom(t, alice) },
			want:                  true,
			serverCurrentlyInRoom: true,
		},
		{
			name:       "allowed due to pending invite",
			serverName: "test2",
			roomFunc: func() *test.Room {
				bob := test.User{ID: "@bob:test2"}
				r := test.NewRoom(t, alice, test.RoomHistoryVisibility(gomatrixserverlib.HistoryVisibilityInvited))
				r.CreateAndInsert(t, alice, spec.MRoomMember, map[string]interface{}{
					"membership": spec.Invite,
				}, test.WithStateKey(bob.ID))
				return r
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.roomFunc == nil {
				t.Fatalf("missing roomFunc")
			}
			var authEvents []gomatrixserverlib.PDU
			for _, ev := range tt.roomFunc().Events() {
				authEvents = append(authEvents, ev.PDU)
			}

			if got := IsServerAllowed(context.Background(), &FakeQuerier{}, tt.serverName, tt.serverCurrentlyInRoom, authEvents); got != tt.want {
				t.Errorf("IsServerAllowed() = %v, want %v", got, tt.want)
			}
		})
	}
}
