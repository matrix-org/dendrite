// Copyright 2020 The Matrix.org Foundation C.I.C.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package acls

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"strings"
	"sync"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"
)

type ServerACLDatabase interface {
	// GetKnownRooms returns a list of all rooms we know about.
	GetKnownRooms(ctx context.Context) ([]string, error)
	// GetStateEvent returns the state event of a given type for a given room with a given state key
	// If no event could be found, returns nil
	// If there was an issue during the retrieval, returns an error
	GetStateEvent(ctx context.Context, roomID, evType, stateKey string) (*gomatrixserverlib.HeaderedEvent, error)
}

type ServerACLs struct {
	acls      map[string]*serverACL // room ID -> ACL
	aclsMutex sync.RWMutex          // protects the above
}

func NewServerACLs(db ServerACLDatabase) *ServerACLs {
	ctx := context.TODO()
	acls := &ServerACLs{
		acls: make(map[string]*serverACL),
	}
	// Look up all of the rooms that the current state server knows about.
	rooms, err := db.GetKnownRooms(ctx)
	if err != nil {
		logrus.WithError(err).Fatalf("Failed to get known rooms")
	}
	// For each room, let's see if we have a server ACL state event. If we
	// do then we'll process it into memory so that we have the regexes to
	// hand.
	for _, room := range rooms {
		state, err := db.GetStateEvent(ctx, room, "m.room.server_acl", "")
		if err != nil {
			logrus.WithError(err).Errorf("Failed to get server ACLs for room %q", room)
			continue
		}
		if state != nil {
			acls.OnServerACLUpdate(state.Event)
		}
	}
	return acls
}

type ServerACL struct {
	Allowed         []string `json:"allow"`
	Denied          []string `json:"deny"`
	AllowIPLiterals bool     `json:"allow_ip_literals"`
}

type serverACL struct {
	ServerACL
	allowedRegexes []*regexp.Regexp
	deniedRegexes  []*regexp.Regexp
}

func compileACLRegex(orig string) (*regexp.Regexp, error) {
	escaped := regexp.QuoteMeta(orig)
	escaped = strings.Replace(escaped, "\\?", ".", -1)
	escaped = strings.Replace(escaped, "\\*", ".*", -1)
	return regexp.Compile(escaped)
}

func (s *ServerACLs) OnServerACLUpdate(state *gomatrixserverlib.Event) {
	acls := &serverACL{}
	if err := json.Unmarshal(state.Content(), &acls.ServerACL); err != nil {
		logrus.WithError(err).Errorf("Failed to unmarshal state content for server ACLs")
		return
	}
	// The spec calls only for * (zero or more chars) and ? (exactly one char)
	// to be supported as wildcard components, so we will escape all of the regex
	// special characters and then replace * and ? with their regex counterparts.
	// https://matrix.org/docs/spec/client_server/r0.6.1#m-room-server-acl
	for _, orig := range acls.Allowed {
		if expr, err := compileACLRegex(orig); err != nil {
			logrus.WithError(err).Errorf("Failed to compile allowed regex")
		} else {
			acls.allowedRegexes = append(acls.allowedRegexes, expr)
		}
	}
	for _, orig := range acls.Denied {
		if expr, err := compileACLRegex(orig); err != nil {
			logrus.WithError(err).Errorf("Failed to compile denied regex")
		} else {
			acls.deniedRegexes = append(acls.deniedRegexes, expr)
		}
	}
	logrus.WithFields(logrus.Fields{
		"allow_ip_literals": acls.AllowIPLiterals,
		"num_allowed":       len(acls.allowedRegexes),
		"num_denied":        len(acls.deniedRegexes),
	}).Debugf("Updating server ACLs for %q", state.RoomID())
	s.aclsMutex.Lock()
	defer s.aclsMutex.Unlock()
	s.acls[state.RoomID()] = acls
}

func (s *ServerACLs) IsServerBannedFromRoom(serverName gomatrixserverlib.ServerName, roomID string) bool {
	s.aclsMutex.RLock()
	// First of all check if we have an ACL for this room. If we don't then
	// no servers are banned from the room.
	acls, ok := s.acls[roomID]
	if !ok {
		s.aclsMutex.RUnlock()
		return false
	}
	s.aclsMutex.RUnlock()
	// Split the host and port apart. This is because the spec calls on us to
	// validate the hostname only in cases where the port is also present.
	if serverNameOnly, _, err := net.SplitHostPort(string(serverName)); err == nil {
		serverName = gomatrixserverlib.ServerName(serverNameOnly)
	}
	// Check if the hostname is an IPv4 or IPv6 literal. We cheat here by adding
	// a /0 prefix length just to trick ParseCIDR into working. If we find that
	// the server is an IP literal and we don't allow those then stop straight
	// away.
	if _, _, err := net.ParseCIDR(fmt.Sprintf("%s/0", serverName)); err == nil {
		if !acls.AllowIPLiterals {
			return true
		}
	}
	// Check if the hostname matches one of the denied regexes. If it does then
	// the server is banned from the room.
	for _, expr := range acls.deniedRegexes {
		if expr.MatchString(string(serverName)) {
			return true
		}
	}
	// Check if the hostname matches one of the allowed regexes. If it does then
	// the server is NOT banned from the room.
	for _, expr := range acls.allowedRegexes {
		if expr.MatchString(string(serverName)) {
			return false
		}
	}
	// If we've got to this point then we haven't matched any regexes or an IP
	// hostname if disallowed. The spec calls for default-deny here.
	return true
}
