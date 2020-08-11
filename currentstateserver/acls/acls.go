package acls

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"strings"
	"sync"

	"github.com/matrix-org/dendrite/currentstateserver/storage"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"
)

type ServerACLs struct {
	acls      map[string]*serverACL // room ID -> ACL
	aclsMutex sync.RWMutex          // protects the above
}

func NewServerACLs(db storage.Database) *ServerACLs {
	ctx := context.TODO()
	acls := &ServerACLs{
		acls: make(map[string]*serverACL),
	}
	rooms, err := db.GetKnownRooms(ctx)
	if err != nil {
		logrus.WithError(err).Fatalf("Failed to get known rooms")
	}
	for _, room := range rooms {
		state, err := db.GetStateEvent(ctx, room, "m.room.server_acl", "")
		if err != nil {
			logrus.WithError(err).Errorf("Failed to get server ACLs for room %q", room)
			continue
		}
		if state != nil {
			acls.OnServerACLUpdate(&state.Event)
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

func (s *ServerACLs) OnServerACLUpdate(state *gomatrixserverlib.Event) {
	acls := &serverACL{}
	if err := json.Unmarshal(state.Content(), &acls.ServerACL); err != nil {
		return
	}
	for _, orig := range acls.Allowed {
		escaped := regexp.QuoteMeta(orig)
		escaped = strings.Replace(escaped, "\\?", "(.)", -1)
		escaped = strings.Replace(escaped, "\\*", "(.*)", -1)
		if expr, err := regexp.Compile(escaped); err == nil {
			acls.allowedRegexes = append(acls.allowedRegexes, expr)
		}
	}
	for _, orig := range acls.Denied {
		escaped := regexp.QuoteMeta(orig)
		escaped = strings.Replace(escaped, "\\?", "(.)", -1)
		escaped = strings.Replace(escaped, "\\*", "(.*)", -1)
		if expr, err := regexp.Compile(escaped); err == nil {
			acls.deniedRegexes = append(acls.deniedRegexes, expr)
		}
	}
	logrus.WithFields(logrus.Fields{
		"allow_ip_literals": acls.AllowIPLiterals,
		"num_allowed":       len(acls.allowedRegexes),
		"num_denied":        len(acls.deniedRegexes),
	}).Infof("Updating server ACLs for %q", state.RoomID())
	s.aclsMutex.Lock()
	defer s.aclsMutex.Unlock()
	s.acls[state.RoomID()] = acls
}

func (s *ServerACLs) IsServerBannedFromRoom(serverNameAndPort gomatrixserverlib.ServerName, roomID string) bool {
	s.aclsMutex.RLock()
	acls, ok := s.acls[roomID]
	if !ok {
		s.aclsMutex.RUnlock()
		return false
	}
	s.aclsMutex.RUnlock()
	serverName, _, err := net.SplitHostPort(string(serverNameAndPort))
	if err != nil {
		return true
	}
	if _, _, err := net.ParseCIDR(fmt.Sprintf("%s/0", serverName)); err == nil {
		if !acls.AllowIPLiterals {
			return true
		}
	}
	for _, expr := range acls.deniedRegexes {
		if expr.MatchString(string(serverName)) {
			return true
		}
	}
	for _, expr := range acls.allowedRegexes {
		if expr.MatchString(string(serverName)) {
			return false
		}
	}
	return true
}
