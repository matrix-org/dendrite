package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"strings"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"
)

// GetEvent returns the current state event in the room or nil.
func GetEvent(ctx context.Context, stateAPI CurrentStateInternalAPI, roomID string, tuple gomatrixserverlib.StateKeyTuple) *gomatrixserverlib.HeaderedEvent {
	var res QueryCurrentStateResponse
	err := stateAPI.QueryCurrentState(ctx, &QueryCurrentStateRequest{
		RoomID:      roomID,
		StateTuples: []gomatrixserverlib.StateKeyTuple{tuple},
	}, &res)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("Failed to QueryCurrentState")
		return nil
	}
	ev, ok := res.StateEvents[tuple]
	if ok {
		return ev
	}
	return nil
}

// IsServerBannedFromRoom returns whether the server is banned from a room by server ACLs.
func IsServerBannedFromRoom(ctx context.Context, stateAPI CurrentStateInternalAPI, roomID string, serverName gomatrixserverlib.ServerName) bool {
	tuple := gomatrixserverlib.StateKeyTuple{
		EventType: "m.room.server_acl",
		StateKey:  "",
	}
	req := &QueryCurrentStateRequest{
		RoomID:      roomID,
		StateTuples: []gomatrixserverlib.StateKeyTuple{tuple},
	}
	res := &QueryCurrentStateResponse{}
	if err := stateAPI.QueryCurrentState(ctx, req, res); err != nil {
		logrus.WithError(err).Error("Failed to query current state for server ACL")
		return true
	}
	state, ok := res.StateEvents[tuple]
	if !ok {
		return false
	}
	acls := struct {
		Allowed         []string `json:"allow"`
		Denied          []string `json:"deny"`
		AllowIPLiterals bool     `json:"allow_ip_literals"`
	}{}
	if err := json.Unmarshal(state.Content(), &acls); err != nil {
		return true
	}
	// First, check to see if this is an IP literal.
	if _, _, err := net.ParseCIDR(fmt.Sprintf("%s/0", serverName)); err == nil {
		if !acls.AllowIPLiterals {
			return true
		}
	}
	// Next, build up a list of regular expressions for allowed and denied.
	allowed := []*regexp.Regexp{}
	denied := []*regexp.Regexp{}
	for _, orig := range acls.Allowed {
		escaped := regexp.QuoteMeta(orig)
		escaped = strings.Replace(escaped, "\\?", "(.)", -1)
		escaped = strings.Replace(escaped, "\\*", "(.*)", -1)
		if expr, err := regexp.Compile(escaped); err == nil {
			allowed = append(allowed, expr)
		}
	}
	for _, orig := range acls.Denied {
		escaped := regexp.QuoteMeta(orig)
		escaped = strings.Replace(escaped, "\\?", "(.)", -1)
		escaped = strings.Replace(escaped, "\\*", "(.*)", -1)
		if expr, err := regexp.Compile(escaped); err == nil {
			denied = append(denied, expr)
		}
	}
	// Now see if we match any of the denied hosts.
	for _, expr := range denied {
		if expr.MatchString(string(serverName)) {
			return true
		}
	}
	// Finally, see if we match any of the allowed hosts.
	for _, expr := range allowed {
		if expr.MatchString(string(serverName)) {
			return false
		}
	}
	// Failing all else deny.
	return true
}

// PopulatePublicRooms extracts PublicRoom information for all the provided room IDs. The IDs are not checked to see if they are visible in the
// published room directory.
// due to lots of switches
// nolint:gocyclo
func PopulatePublicRooms(ctx context.Context, roomIDs []string, stateAPI CurrentStateInternalAPI) ([]gomatrixserverlib.PublicRoom, error) {
	avatarTuple := gomatrixserverlib.StateKeyTuple{EventType: "m.room.avatar", StateKey: ""}
	nameTuple := gomatrixserverlib.StateKeyTuple{EventType: "m.room.name", StateKey: ""}
	canonicalTuple := gomatrixserverlib.StateKeyTuple{EventType: gomatrixserverlib.MRoomCanonicalAlias, StateKey: ""}
	topicTuple := gomatrixserverlib.StateKeyTuple{EventType: "m.room.topic", StateKey: ""}
	guestTuple := gomatrixserverlib.StateKeyTuple{EventType: "m.room.guest_access", StateKey: ""}
	visibilityTuple := gomatrixserverlib.StateKeyTuple{EventType: gomatrixserverlib.MRoomHistoryVisibility, StateKey: ""}
	joinRuleTuple := gomatrixserverlib.StateKeyTuple{EventType: gomatrixserverlib.MRoomJoinRules, StateKey: ""}

	var stateRes QueryBulkStateContentResponse
	err := stateAPI.QueryBulkStateContent(ctx, &QueryBulkStateContentRequest{
		RoomIDs:        roomIDs,
		AllowWildcards: true,
		StateTuples: []gomatrixserverlib.StateKeyTuple{
			nameTuple, canonicalTuple, topicTuple, guestTuple, visibilityTuple, joinRuleTuple, avatarTuple,
			{EventType: gomatrixserverlib.MRoomMember, StateKey: "*"},
		},
	}, &stateRes)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("QueryBulkStateContent failed")
		return nil, err
	}
	chunk := make([]gomatrixserverlib.PublicRoom, len(roomIDs))
	i := 0
	for roomID, data := range stateRes.Rooms {
		pub := gomatrixserverlib.PublicRoom{
			RoomID: roomID,
		}
		joinCount := 0
		var joinRule, guestAccess string
		for tuple, contentVal := range data {
			if tuple.EventType == gomatrixserverlib.MRoomMember && contentVal == "join" {
				joinCount++
				continue
			}
			switch tuple {
			case avatarTuple:
				pub.AvatarURL = contentVal
			case nameTuple:
				pub.Name = contentVal
			case topicTuple:
				pub.Topic = contentVal
			case canonicalTuple:
				pub.CanonicalAlias = contentVal
			case visibilityTuple:
				pub.WorldReadable = contentVal == "world_readable"
			// need both of these to determine whether guests can join
			case joinRuleTuple:
				joinRule = contentVal
			case guestTuple:
				guestAccess = contentVal
			}
		}
		if joinRule == gomatrixserverlib.Public && guestAccess == "can_join" {
			pub.GuestCanJoin = true
		}
		pub.JoinedMembersCount = joinCount
		chunk[i] = pub
		i++
	}
	return chunk, nil
}
