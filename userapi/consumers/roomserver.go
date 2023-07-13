package consumers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/tidwall/gjson"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/nats-io/nats.go"
	log "github.com/sirupsen/logrus"

	"github.com/matrix-org/dendrite/internal/eventutil"
	"github.com/matrix-org/dendrite/internal/pushgateway"
	"github.com/matrix-org/dendrite/internal/pushrules"
	rsapi "github.com/matrix-org/dendrite/roomserver/api"
	rstypes "github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/setup/process"
	"github.com/matrix-org/dendrite/syncapi/synctypes"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/dendrite/userapi/producers"
	"github.com/matrix-org/dendrite/userapi/storage"
	"github.com/matrix-org/dendrite/userapi/storage/tables"
	userAPITypes "github.com/matrix-org/dendrite/userapi/types"
	"github.com/matrix-org/dendrite/userapi/util"
)

type OutputRoomEventConsumer struct {
	ctx          context.Context
	cfg          *config.UserAPI
	rsAPI        rsapi.UserRoomserverAPI
	jetstream    nats.JetStreamContext
	durable      string
	db           storage.UserDatabase
	topic        string
	pgClient     pushgateway.Client
	syncProducer *producers.SyncAPI
	msgCounts    map[spec.ServerName]userAPITypes.MessageStats
	roomCounts   map[spec.ServerName]map[string]bool // map from serverName to map from rommID to "isEncrypted"
	lastUpdate   time.Time
	countsLock   sync.Mutex
	serverName   spec.ServerName
}

func NewOutputRoomEventConsumer(
	process *process.ProcessContext,
	cfg *config.UserAPI,
	js nats.JetStreamContext,
	store storage.UserDatabase,
	pgClient pushgateway.Client,
	rsAPI rsapi.UserRoomserverAPI,
	syncProducer *producers.SyncAPI,
) *OutputRoomEventConsumer {
	return &OutputRoomEventConsumer{
		ctx:          process.Context(),
		cfg:          cfg,
		jetstream:    js,
		db:           store,
		durable:      cfg.Matrix.JetStream.Durable("UserAPIRoomServerConsumer"),
		topic:        cfg.Matrix.JetStream.Prefixed(jetstream.OutputRoomEvent),
		pgClient:     pgClient,
		rsAPI:        rsAPI,
		syncProducer: syncProducer,
		msgCounts:    map[spec.ServerName]userAPITypes.MessageStats{},
		roomCounts:   map[spec.ServerName]map[string]bool{},
		lastUpdate:   time.Now(),
		countsLock:   sync.Mutex{},
		serverName:   cfg.Matrix.ServerName,
	}
}

func (s *OutputRoomEventConsumer) Start() error {
	if err := jetstream.JetStreamConsumer(
		s.ctx, s.jetstream, s.topic, s.durable, 1,
		s.onMessage, nats.DeliverAll(), nats.ManualAck(),
	); err != nil {
		return err
	}
	return nil
}

func (s *OutputRoomEventConsumer) onMessage(ctx context.Context, msgs []*nats.Msg) bool {
	msg := msgs[0] // Guaranteed to exist if onMessage is called
	// Only handle events we care about
	if rsapi.OutputType(msg.Header.Get(jetstream.RoomEventType)) != rsapi.OutputTypeNewRoomEvent {
		return true
	}
	var output rsapi.OutputEvent
	if err := json.Unmarshal(msg.Data, &output); err != nil {
		// If the message was invalid, log it and move on to the next message in the stream
		log.WithError(err).Errorf("roomserver output log: message parse failure")
		return true
	}
	event := output.NewRoomEvent.Event
	if event == nil {
		log.Errorf("userapi consumer: expected event")
		return true
	}

	if s.cfg.Matrix.ReportStats.Enabled {
		go s.storeMessageStats(ctx, event.Type(), string(event.SenderID()), event.RoomID())
	}

	log.WithFields(log.Fields{
		"event_id":   event.EventID(),
		"event_type": event.Type(),
	}).Tracef("Received message from roomserver: %#v", output)

	metadata, err := msg.Metadata()
	if err != nil {
		return true
	}

	if err := s.processMessage(ctx, event, uint64(spec.AsTimestamp(metadata.Timestamp))); err != nil {
		log.WithFields(log.Fields{
			"event_id": event.EventID(),
		}).WithError(err).Errorf("userapi consumer: process room event failure")
	}

	return true
}

func (s *OutputRoomEventConsumer) storeMessageStats(ctx context.Context, eventType, eventSender, roomID string) {
	s.countsLock.Lock()
	defer s.countsLock.Unlock()

	// reset the roomCounts on a day change
	if s.lastUpdate.Day() != time.Now().Day() {
		s.roomCounts[s.serverName] = make(map[string]bool)
		s.lastUpdate = time.Now()
	}

	_, sender, err := gomatrixserverlib.SplitID('@', eventSender)
	if err != nil {
		return
	}
	msgCount := s.msgCounts[s.serverName]
	roomCount := s.roomCounts[s.serverName]
	if roomCount == nil {
		roomCount = make(map[string]bool)
	}
	switch eventType {
	case "m.room.message":
		roomCount[roomID] = false
		msgCount.Messages++
		if sender == s.serverName {
			msgCount.SentMessages++
		}
	case "m.room.encrypted":
		roomCount[roomID] = true
		msgCount.MessagesE2EE++
		if sender == s.serverName {
			msgCount.SentMessagesE2EE++
		}
	default:
		return
	}
	s.msgCounts[s.serverName] = msgCount
	s.roomCounts[s.serverName] = roomCount

	for serverName, stats := range s.msgCounts {
		var normalRooms, encryptedRooms int64 = 0, 0
		for _, isEncrypted := range s.roomCounts[s.serverName] {
			if isEncrypted {
				encryptedRooms++
			} else {
				normalRooms++
			}
		}
		err := s.db.UpsertDailyRoomsMessages(ctx, serverName, stats, normalRooms, encryptedRooms)
		if err != nil {
			log.WithError(err).Errorf("failed to upsert daily messages")
		}
		// Clear stats if we successfully stored it
		if err == nil {
			stats.Messages = 0
			stats.SentMessages = 0
			stats.MessagesE2EE = 0
			stats.SentMessagesE2EE = 0
			s.msgCounts[serverName] = stats
		}
	}
}

func (s *OutputRoomEventConsumer) handleRoomUpgrade(ctx context.Context, oldRoomID, newRoomID string, localMembers []*localMembership, roomSize int) error {
	for _, membership := range localMembers {
		// Copy any existing push rules from old -> new room
		if err := s.copyPushrules(ctx, oldRoomID, newRoomID, membership.Localpart, membership.Domain); err != nil {
			return err
		}

		// preserve m.direct room state
		if err := s.updateMDirect(ctx, oldRoomID, newRoomID, membership.Localpart, membership.Domain, roomSize); err != nil {
			return err
		}

		// copy existing m.tag entries, if any
		if err := s.copyTags(ctx, oldRoomID, newRoomID, membership.Localpart, membership.Domain); err != nil {
			return err
		}
	}
	return nil
}

func (s *OutputRoomEventConsumer) copyPushrules(ctx context.Context, oldRoomID, newRoomID string, localpart string, serverName spec.ServerName) error {
	pushRules, err := s.db.QueryPushRules(ctx, localpart, serverName)
	if err != nil {
		return fmt.Errorf("failed to query pushrules for user: %w", err)
	}
	if pushRules == nil {
		return nil
	}

	for _, roomRule := range pushRules.Global.Room {
		if roomRule.RuleID != oldRoomID {
			continue
		}
		cpRool := *roomRule
		cpRool.RuleID = newRoomID
		pushRules.Global.Room = append(pushRules.Global.Room, &cpRool)
		rules, err := json.Marshal(pushRules)
		if err != nil {
			return err
		}
		if err = s.db.SaveAccountData(ctx, localpart, serverName, "", "m.push_rules", rules); err != nil {
			return fmt.Errorf("failed to update pushrules: %w", err)
		}
	}
	return nil
}

// updateMDirect copies the "is_direct" flag from oldRoomID to newROomID
func (s *OutputRoomEventConsumer) updateMDirect(ctx context.Context, oldRoomID, newRoomID, localpart string, serverName spec.ServerName, roomSize int) error {
	// this is most likely not a DM, so skip updating m.direct state
	if roomSize > 2 {
		return nil
	}
	// Get direct message state
	directChatsRaw, err := s.db.GetAccountDataByType(ctx, localpart, serverName, "", "m.direct")
	if err != nil {
		return fmt.Errorf("failed to get m.direct from database: %w", err)
	}
	directChats := gjson.ParseBytes(directChatsRaw)
	newDirectChats := make(map[string][]string)
	// iterate over all userID -> roomIDs
	directChats.ForEach(func(userID, roomIDs gjson.Result) bool {
		var found bool
		for _, roomID := range roomIDs.Array() {
			newDirectChats[userID.Str] = append(newDirectChats[userID.Str], roomID.Str)
			// add the new roomID to m.direct
			if roomID.Str == oldRoomID {
				found = true
				newDirectChats[userID.Str] = append(newDirectChats[userID.Str], newRoomID)
			}
		}
		// Only hit the database if we found the old room as a DM for this user
		if found {
			var data []byte
			data, err = json.Marshal(newDirectChats)
			if err != nil {
				return true
			}
			if err = s.db.SaveAccountData(ctx, localpart, serverName, "", "m.direct", data); err != nil {
				return true
			}
		}
		return true
	})
	if err != nil {
		return fmt.Errorf("failed to update m.direct state")
	}
	return nil
}

func (s *OutputRoomEventConsumer) copyTags(ctx context.Context, oldRoomID, newRoomID, localpart string, serverName spec.ServerName) error {
	tag, err := s.db.GetAccountDataByType(ctx, localpart, serverName, oldRoomID, "m.tag")
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if tag == nil {
		return nil
	}
	return s.db.SaveAccountData(ctx, localpart, serverName, newRoomID, "m.tag", tag)
}

func (s *OutputRoomEventConsumer) processMessage(ctx context.Context, event *rstypes.HeaderedEvent, streamPos uint64) error {
	members, roomSize, err := s.localRoomMembers(ctx, event.RoomID())
	if err != nil {
		return fmt.Errorf("s.localRoomMembers: %w", err)
	}

	switch {
	case event.Type() == spec.MRoomMember:
		sender := spec.UserID{}
		validRoomID, roomErr := spec.NewRoomID(event.RoomID())
		if roomErr != nil {
			return roomErr
		}
		userID, queryErr := s.rsAPI.QueryUserIDForSender(ctx, *validRoomID, event.SenderID())
		if queryErr == nil && userID != nil {
			sender = *userID
		}

		sk := event.StateKey()
		if sk != nil && *sk != "" {
			skUserID, queryErr := s.rsAPI.QueryUserIDForSender(ctx, *validRoomID, spec.SenderID(*sk))
			if queryErr == nil && skUserID != nil {
				skString := skUserID.String()
				sk = &skString
			} else {
				return fmt.Errorf("queryUserIDForSender: userID unknown for %s", *sk)
			}
		}
		cevent := synctypes.ToClientEvent(event, synctypes.FormatAll, sender, sk)
		var member *localMembership
		member, err = newLocalMembership(&cevent)
		if err != nil {
			return fmt.Errorf("newLocalMembership: %w", err)
		}
		if member.Membership == spec.Invite && member.Domain == s.cfg.Matrix.ServerName {
			// localRoomMembers only adds joined members. An invite
			// should also be pushed to the target user.
			members = append(members, member)
		}
	case event.Type() == "m.room.tombstone" && event.StateKeyEquals(""):
		// Handle room upgrades
		oldRoomID := event.RoomID()
		newRoomID := gjson.GetBytes(event.Content(), "replacement_room").Str
		if err = s.handleRoomUpgrade(ctx, oldRoomID, newRoomID, members, roomSize); err != nil {
			// while inconvenient, this shouldn't stop us from sending push notifications
			log.WithError(err).Errorf("UserAPI: failed to handle room upgrade for users")
		}

	}

	// TODO: run in parallel with localRoomMembers.
	roomName, err := s.roomName(ctx, event)
	if err != nil {
		return fmt.Errorf("s.roomName: %w", err)
	}

	log.WithFields(log.Fields{
		"event_id":    event.EventID(),
		"room_id":     event.RoomID(),
		"num_members": len(members),
		"room_size":   roomSize,
	}).Tracef("Notifying members")

	// Notification.UserIsTarget is a per-member field, so we
	// cannot group all users in a single request.
	//
	// TODO: does it have to be set? It's not required, and
	// removing it means we can send all notifications to
	// e.g. Element's Push gateway in one go.
	for _, mem := range members {
		if err := s.notifyLocal(ctx, event, mem, roomSize, roomName, streamPos); err != nil {
			log.WithFields(log.Fields{
				"localpart": mem.Localpart,
			}).WithError(err).Error("Unable to push to local user")
			continue
		}
	}

	return nil
}

type localMembership struct {
	gomatrixserverlib.MemberContent
	UserID    string
	Localpart string
	Domain    spec.ServerName
}

func newLocalMembership(event *synctypes.ClientEvent) (*localMembership, error) {
	if event.StateKey == nil {
		return nil, fmt.Errorf("missing state_key")
	}

	var member localMembership
	if err := json.Unmarshal(event.Content, &member.MemberContent); err != nil {
		return nil, err
	}

	localpart, domain, err := gomatrixserverlib.SplitID('@', *event.StateKey)
	if err != nil {
		return nil, err
	}

	member.UserID = *event.StateKey
	member.Localpart = localpart
	member.Domain = domain
	return &member, nil
}

// localRoomMembers fetches the current local members of a room, and
// the total number of members.
func (s *OutputRoomEventConsumer) localRoomMembers(ctx context.Context, roomID string) ([]*localMembership, int, error) {
	// Get only locally joined users to avoid unmarshalling and caching
	// membership events we only use to calculate the room size.
	req := &rsapi.QueryMembershipsForRoomRequest{
		RoomID:     roomID,
		JoinedOnly: true,
		LocalOnly:  true,
	}
	var res rsapi.QueryMembershipsForRoomResponse
	if err := s.rsAPI.QueryMembershipsForRoom(ctx, req, &res); err != nil {
		return nil, 0, err
	}

	// Since we only queried locally joined users above,
	// we also need to ask the roomserver about the joined user count.
	totalCount, err := s.rsAPI.JoinedUserCount(ctx, roomID)
	if err != nil {
		return nil, 0, err
	}

	var members []*localMembership
	for _, event := range res.JoinEvents {
		// Filter out invalid join events
		if event.StateKey == nil {
			continue
		}
		if *event.StateKey == "" {
			continue
		}
		// We're going to trust the Query from above to really just return
		// local users
		member, err := newLocalMembership(&event)
		if err != nil {
			log.WithError(err).Errorf("Parsing MemberContent")
			continue
		}

		members = append(members, member)
	}

	return members, totalCount, nil
}

// roomName returns the name in the event (if type==m.room.name), or
// looks it up in roomserver. If there is no name,
// m.room.canonical_alias is consulted. Returns an empty string if the
// room has no name.
func (s *OutputRoomEventConsumer) roomName(ctx context.Context, event *rstypes.HeaderedEvent) (string, error) {
	if event.Type() == spec.MRoomName {
		name, err := unmarshalRoomName(event)
		if err != nil {
			return "", err
		}

		if name != "" {
			return name, nil
		}
	}

	req := &rsapi.QueryCurrentStateRequest{
		RoomID:      event.RoomID(),
		StateTuples: []gomatrixserverlib.StateKeyTuple{roomNameTuple, canonicalAliasTuple},
	}
	var res rsapi.QueryCurrentStateResponse

	if err := s.rsAPI.QueryCurrentState(ctx, req, &res); err != nil {
		return "", nil
	}

	if eventS := res.StateEvents[roomNameTuple]; eventS != nil {
		return unmarshalRoomName(eventS)
	}

	if event.Type() == spec.MRoomCanonicalAlias {
		alias, err := unmarshalCanonicalAlias(event)
		if err != nil {
			return "", err
		}

		if alias != "" {
			return alias, nil
		}
	}

	if event = res.StateEvents[canonicalAliasTuple]; event != nil {
		return unmarshalCanonicalAlias(event)
	}

	return "", nil
}

var (
	canonicalAliasTuple = gomatrixserverlib.StateKeyTuple{EventType: spec.MRoomCanonicalAlias}
	roomNameTuple       = gomatrixserverlib.StateKeyTuple{EventType: spec.MRoomName}
)

func unmarshalRoomName(event *rstypes.HeaderedEvent) (string, error) {
	var nc eventutil.NameContent
	if err := json.Unmarshal(event.Content(), &nc); err != nil {
		return "", fmt.Errorf("unmarshaling NameContent: %w", err)
	}

	return nc.Name, nil
}

func unmarshalCanonicalAlias(event *rstypes.HeaderedEvent) (string, error) {
	var cac eventutil.CanonicalAliasContent
	if err := json.Unmarshal(event.Content(), &cac); err != nil {
		return "", fmt.Errorf("unmarshaling CanonicalAliasContent: %w", err)
	}

	return cac.Alias, nil
}

// notifyLocal finds the right push actions for a local user, given an event.
func (s *OutputRoomEventConsumer) notifyLocal(ctx context.Context, event *rstypes.HeaderedEvent, mem *localMembership, roomSize int, roomName string, streamPos uint64) error {
	actions, err := s.evaluatePushRules(ctx, event, mem, roomSize)
	if err != nil {
		return fmt.Errorf("s.evaluatePushRules: %w", err)
	}
	a, tweaks, err := pushrules.ActionsToTweaks(actions)
	if err != nil {
		return fmt.Errorf("pushrules.ActionsToTweaks: %w", err)
	}
	// TODO: support coalescing.
	if a != pushrules.NotifyAction && a != pushrules.CoalesceAction {
		log.WithFields(log.Fields{
			"event_id":  event.EventID(),
			"room_id":   event.RoomID(),
			"localpart": mem.Localpart,
		}).Tracef("Push rule evaluation rejected the event")
		return nil
	}

	devicesByURLAndFormat, profileTag, err := s.localPushDevices(ctx, mem.Localpart, mem.Domain, tweaks)
	if err != nil {
		return fmt.Errorf("s.localPushDevices: %w", err)
	}

	sender := spec.UserID{}
	validRoomID, err := spec.NewRoomID(event.RoomID())
	if err != nil {
		return err
	}
	userID, err := s.rsAPI.QueryUserIDForSender(ctx, *validRoomID, event.SenderID())
	if err == nil && userID != nil {
		sender = *userID
	}

	sk := event.StateKey()
	if sk != nil && *sk != "" {
		skUserID, queryErr := s.rsAPI.QueryUserIDForSender(ctx, *validRoomID, spec.SenderID(*event.StateKey()))
		if queryErr == nil && skUserID != nil {
			skString := skUserID.String()
			sk = &skString
		}
	}
	n := &api.Notification{
		Actions: actions,
		// UNSPEC: the spec doesn't say this is a ClientEvent, but the
		// fields seem to match. room_id should be missing, which
		// matches the behaviour of FormatSync.
		Event: synctypes.ToClientEvent(event, synctypes.FormatSync, sender, sk),
		// TODO: this is per-device, but it's not part of the primary
		// key. So inserting one notification per profile tag doesn't
		// make sense. What is this supposed to be? Sytests require it
		// to "work", but they only use a single device.
		ProfileTag: profileTag,
		RoomID:     event.RoomID(),
		TS:         spec.AsTimestamp(time.Now()),
	}
	if err = s.db.InsertNotification(ctx, mem.Localpart, mem.Domain, event.EventID(), streamPos, tweaks, n); err != nil {
		return fmt.Errorf("s.db.InsertNotification: %w", err)
	}

	if err = s.syncProducer.GetAndSendNotificationData(ctx, mem.UserID, event.RoomID()); err != nil {
		return fmt.Errorf("s.syncProducer.GetAndSendNotificationData: %w", err)
	}

	// We do this after InsertNotification. Thus, this should always return >=1.
	userNumUnreadNotifs, err := s.db.GetNotificationCount(ctx, mem.Localpart, mem.Domain, tables.AllNotifications)
	if err != nil {
		return fmt.Errorf("s.db.GetNotificationCount: %w", err)
	}

	log.WithFields(log.Fields{
		"event_id":   event.EventID(),
		"room_id":    event.RoomID(),
		"localpart":  mem.Localpart,
		"num_urls":   len(devicesByURLAndFormat),
		"num_unread": userNumUnreadNotifs,
	}).Trace("Notifying single member")

	// Push gateways are out of our control, and we cannot risk
	// looking up the server on a misbehaving push gateway. Each user
	// receives a goroutine now that all internal API calls have been
	// made.
	//
	// TODO: think about bounding this to one per user, and what
	// ordering guarantees we must provide.
	go func() {
		// This background processing cannot be tied to a request.
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		var rejected []*pushgateway.Device
		for url, fmts := range devicesByURLAndFormat {
			for format, devices := range fmts {
				// TODO: support "email".
				if !strings.HasPrefix(url, "http") {
					continue
				}

				// UNSPEC: the specification suggests there can be
				// more than one device per request. There is at least
				// one Sytest that expects one HTTP request per
				// device, rather than per URL. For now, we must
				// notify each one separately.
				for _, dev := range devices {
					rej, err := s.notifyHTTP(ctx, event, url, format, []*pushgateway.Device{dev}, mem.Localpart, roomName, int(userNumUnreadNotifs))
					if err != nil {
						log.WithFields(log.Fields{
							"event_id":  event.EventID(),
							"localpart": mem.Localpart,
						}).WithError(err).Errorf("Unable to notify HTTP pusher")
						continue
					}
					rejected = append(rejected, rej...)
				}
			}
		}

		if len(rejected) > 0 {
			s.deleteRejectedPushers(ctx, rejected, mem.Localpart, mem.Domain)
		}
	}()

	return nil
}

// evaluatePushRules fetches and evaluates the push rules of a local
// user. Returns actions (including dont_notify).
func (s *OutputRoomEventConsumer) evaluatePushRules(ctx context.Context, event *rstypes.HeaderedEvent, mem *localMembership, roomSize int) ([]*pushrules.Action, error) {
	user := ""
	validRoomID, err := spec.NewRoomID(event.RoomID())
	if err != nil {
		return nil, err
	}
	sender, err := s.rsAPI.QueryUserIDForSender(ctx, *validRoomID, event.SenderID())
	if err == nil {
		user = sender.String()
	}
	if user == mem.UserID {
		// SPEC: Homeservers MUST NOT notify the Push Gateway for
		// events that the user has sent themselves.
		return nil, nil
	}

	// Get accountdata to check if the event.Sender() is ignored by mem.LocalPart
	data, err := s.db.GetAccountDataByType(ctx, mem.Localpart, mem.Domain, "", "m.ignored_user_list")
	if err != nil {
		return nil, err
	}
	if data != nil {
		ignored := types.IgnoredUsers{}
		err = json.Unmarshal(data, &ignored)
		if err != nil {
			return nil, err
		}
		if _, ok := ignored.List[sender.String()]; ok {
			return nil, fmt.Errorf("user %s is ignored", sender.String())
		}
	}
	ruleSets, err := s.db.QueryPushRules(ctx, mem.Localpart, mem.Domain)
	if err != nil {
		return nil, err
	}

	ec := &ruleSetEvalContext{
		ctx:      ctx,
		rsAPI:    s.rsAPI,
		mem:      mem,
		roomID:   event.RoomID(),
		roomSize: roomSize,
	}
	eval := pushrules.NewRuleSetEvaluator(ec, &ruleSets.Global)
	rule, err := eval.MatchEvent(event.PDU, func(roomID spec.RoomID, senderID spec.SenderID) (*spec.UserID, error) {
		return s.rsAPI.QueryUserIDForSender(ctx, roomID, senderID)
	})
	if err != nil {
		return nil, err
	}
	if rule == nil {
		// SPEC: If no rules match an event, the homeserver MUST NOT
		// notify the Push Gateway for that event.
		return nil, nil
	}

	log.WithFields(log.Fields{
		"event_id":  event.EventID(),
		"room_id":   event.RoomID(),
		"localpart": mem.Localpart,
		"rule_id":   rule.RuleID,
	}).Trace("Matched a push rule")

	return rule.Actions, nil
}

type ruleSetEvalContext struct {
	ctx      context.Context
	rsAPI    rsapi.UserRoomserverAPI
	mem      *localMembership
	roomID   string
	roomSize int
}

func (rse *ruleSetEvalContext) UserDisplayName() string { return rse.mem.DisplayName }

func (rse *ruleSetEvalContext) RoomMemberCount() (int, error) { return rse.roomSize, nil }

func (rse *ruleSetEvalContext) HasPowerLevel(senderID spec.SenderID, levelKey string) (bool, error) {
	req := &rsapi.QueryLatestEventsAndStateRequest{
		RoomID: rse.roomID,
		StateToFetch: []gomatrixserverlib.StateKeyTuple{
			{EventType: spec.MRoomPowerLevels},
		},
	}
	var res rsapi.QueryLatestEventsAndStateResponse
	if err := rse.rsAPI.QueryLatestEventsAndState(rse.ctx, req, &res); err != nil {
		return false, err
	}
	for _, ev := range res.StateEvents {
		if ev.Type() != spec.MRoomPowerLevels {
			continue
		}

		plc, err := gomatrixserverlib.NewPowerLevelContentFromEvent(ev.PDU)
		if err != nil {
			return false, err
		}
		return plc.UserLevel(senderID) >= plc.NotificationLevel(levelKey), nil
	}
	return true, nil
}

// localPushDevices pushes to the configured devices of a local
// user. The map keys are [url][format].
func (s *OutputRoomEventConsumer) localPushDevices(ctx context.Context, localpart string, serverName spec.ServerName, tweaks map[string]interface{}) (map[string]map[string][]*pushgateway.Device, string, error) {
	pusherDevices, err := util.GetPushDevices(ctx, localpart, serverName, tweaks, s.db)
	if err != nil {
		return nil, "", fmt.Errorf("util.GetPushDevices: %w", err)
	}

	var profileTag string
	devicesByURL := make(map[string]map[string][]*pushgateway.Device, len(pusherDevices))
	for _, pusherDevice := range pusherDevices {
		if profileTag == "" {
			profileTag = pusherDevice.Pusher.ProfileTag
		}

		url := pusherDevice.URL
		if devicesByURL[url] == nil {
			devicesByURL[url] = make(map[string][]*pushgateway.Device, 2)
		}
		devicesByURL[url][pusherDevice.Format] = append(devicesByURL[url][pusherDevice.Format], &pusherDevice.Device)
	}

	return devicesByURL, profileTag, nil
}

// notifyHTTP performs a notificatation to a Push Gateway.
func (s *OutputRoomEventConsumer) notifyHTTP(ctx context.Context, event *rstypes.HeaderedEvent, url, format string, devices []*pushgateway.Device, localpart, roomName string, userNumUnreadNotifs int) ([]*pushgateway.Device, error) {
	logger := log.WithFields(log.Fields{
		"event_id":    event.EventID(),
		"url":         url,
		"localpart":   localpart,
		"num_devices": len(devices),
	})

	var req pushgateway.NotifyRequest
	switch format {
	case "event_id_only":
		req = pushgateway.NotifyRequest{
			Notification: pushgateway.Notification{
				Counts: &pushgateway.Counts{
					Unread: userNumUnreadNotifs,
				},
				Devices: devices,
				EventID: event.EventID(),
				RoomID:  event.RoomID(),
			},
		}

	default:
		validRoomID, err := spec.NewRoomID(event.RoomID())
		if err != nil {
			return nil, err
		}
		sender, err := s.rsAPI.QueryUserIDForSender(ctx, *validRoomID, event.SenderID())
		if err != nil {
			logger.WithError(err).Errorf("Failed to get userID for sender %s", event.SenderID())
			return nil, err
		}
		req = pushgateway.NotifyRequest{
			Notification: pushgateway.Notification{
				Content: event.Content(),
				Counts: &pushgateway.Counts{
					Unread: userNumUnreadNotifs,
				},
				Devices:  devices,
				EventID:  event.EventID(),
				ID:       event.EventID(),
				RoomID:   event.RoomID(),
				RoomName: roomName,
				Sender:   sender.String(),
				Type:     event.Type(),
			},
		}
		if mem, memberErr := event.Membership(); memberErr == nil {
			req.Notification.Membership = mem
		}
		userID, err := spec.NewUserID(fmt.Sprintf("@%s:%s", localpart, s.cfg.Matrix.ServerName), true)
		if err != nil {
			logger.WithError(err).Errorf("Failed to convert local user to userID %s", localpart)
			return nil, err
		}
		roomID, err := spec.NewRoomID(event.RoomID())
		if err != nil {
			logger.WithError(err).Errorf("event roomID is invalid %s", event.RoomID())
			return nil, err
		}

		localSender, err := s.rsAPI.QuerySenderIDForUser(ctx, *roomID, *userID)
		if err != nil {
			logger.WithError(err).Errorf("Failed to get local user senderID for room %s: %s", userID.String(), event.RoomID())
			return nil, err
		}
		if event.StateKey() != nil && *event.StateKey() == string(localSender) {
			req.Notification.UserIsTarget = true
		}
	}

	logger.Tracef("Notifying push gateway %s", url)
	var res pushgateway.NotifyResponse
	if err := s.pgClient.Notify(ctx, url, &req, &res); err != nil {
		logger.WithError(err).Errorf("Failed to notify push gateway %s", url)
		return nil, err
	}
	logger.WithField("num_rejected", len(res.Rejected)).Trace("Push gateway result")

	if len(res.Rejected) == 0 {
		return nil, nil
	}

	devMap := make(map[string]*pushgateway.Device, len(devices))
	for _, d := range devices {
		devMap[d.PushKey] = d
	}
	rejected := make([]*pushgateway.Device, 0, len(res.Rejected))
	for _, pushKey := range res.Rejected {
		d := devMap[pushKey]
		if d != nil {
			rejected = append(rejected, d)
		}
	}

	return rejected, nil
}

// deleteRejectedPushers deletes the pushers associated with the given devices.
func (s *OutputRoomEventConsumer) deleteRejectedPushers(ctx context.Context, devices []*pushgateway.Device, localpart string, serverName spec.ServerName) {
	log.WithFields(log.Fields{
		"localpart":   localpart,
		"app_id0":     devices[0].AppID,
		"num_devices": len(devices),
	}).Warnf("Deleting pushers rejected by the HTTP push gateway")

	for _, d := range devices {
		if err := s.db.RemovePusher(ctx, d.AppID, d.PushKey, localpart, serverName); err != nil {
			log.WithFields(log.Fields{
				"localpart": localpart,
			}).WithError(err).Errorf("Unable to delete rejected pusher")
		}
	}
}
