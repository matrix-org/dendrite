package consumers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/matrix-org/dendrite/internal/eventutil"
	"github.com/matrix-org/dendrite/internal/pushgateway"
	"github.com/matrix-org/dendrite/internal/pushrules"
	rsapi "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/setup/process"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/dendrite/userapi/producers"
	"github.com/matrix-org/dendrite/userapi/storage"
	"github.com/matrix-org/dendrite/userapi/storage/tables"
	"github.com/matrix-org/dendrite/userapi/util"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/nats-io/nats.go"
	log "github.com/sirupsen/logrus"
)

type OutputRoomEventConsumer struct {
	ctx          context.Context
	cfg          *config.UserAPI
	userAPI      api.UserInternalAPI
	rsAPI        rsapi.RoomserverInternalAPI
	jetstream    nats.JetStreamContext
	durable      string
	db           storage.Database
	topic        string
	pgClient     pushgateway.Client
	syncProducer *producers.SyncAPI
}

func NewOutputRoomEventConsumer(
	process *process.ProcessContext,
	cfg *config.UserAPI,
	js nats.JetStreamContext,
	store storage.Database,
	pgClient pushgateway.Client,
	userAPI api.UserInternalAPI,
	rsAPI rsapi.RoomserverInternalAPI,
	syncProducer *producers.SyncAPI,
) *OutputRoomEventConsumer {
	return &OutputRoomEventConsumer{
		ctx:          process.Context(),
		cfg:          cfg,
		jetstream:    js,
		db:           store,
		durable:      cfg.Matrix.JetStream.Durable("UserAPIRoomServerConsumer"),
		topic:        cfg.Matrix.JetStream.TopicFor(jetstream.OutputRoomEvent),
		pgClient:     pgClient,
		userAPI:      userAPI,
		rsAPI:        rsAPI,
		syncProducer: syncProducer,
	}
}

func (s *OutputRoomEventConsumer) Start() error {
	if err := jetstream.JetStreamConsumer(
		s.ctx, s.jetstream, s.topic, s.durable, s.onMessage,
		nats.DeliverAll(), nats.ManualAck(),
	); err != nil {
		return err
	}
	return nil
}

func (s *OutputRoomEventConsumer) onMessage(ctx context.Context, msg *nats.Msg) bool {
	var output rsapi.OutputEvent
	if err := json.Unmarshal(msg.Data, &output); err != nil {
		log.WithError(err).Errorf("pushserver consumer: message parse failure")
		return true
	}

	log.WithFields(log.Fields{
		"event_type": output.Type,
	}).Tracef("Received message from room server: %#v", output)

	switch output.Type {
	case rsapi.OutputTypeNewRoomEvent:
		ev := output.NewRoomEvent.Event
		if err := s.processMessage(ctx, output.NewRoomEvent.Event); err != nil {
			log.WithFields(log.Fields{
				"event_id": ev.EventID(),
				"event":    string(ev.JSON()),
			}).WithError(err).Errorf("pushserver consumer: process room event failure")
		}

	case rsapi.OutputTypeNewInviteEvent:
		ev := output.NewInviteEvent.Event
		if err := s.processMessage(ctx, output.NewInviteEvent.Event); err != nil {
			log.WithFields(log.Fields{
				"event_id": ev.EventID(),
				"event":    string(ev.JSON()),
			}).WithError(err).Errorf("pushserver consumer: process invite event failure")
		}

	default:
		// Ignore old events, peeks, so on.
	}

	return true
}

func (s *OutputRoomEventConsumer) processMessage(ctx context.Context, event *gomatrixserverlib.HeaderedEvent) error {
	log.WithFields(log.Fields{
		"event_type": event.Type(),
	}).Tracef("Received event from room server: %#v", event)

	members, roomSize, err := s.localRoomMembers(ctx, event.RoomID())
	if err != nil {
		return err
	}

	if event.Type() == gomatrixserverlib.MRoomMember {
		cevent := gomatrixserverlib.HeaderedToClientEvent(event, gomatrixserverlib.FormatAll)
		var member *localMembership
		member, err = newLocalMembership(&cevent)
		if err != nil {
			return err
		}
		if member.Membership == gomatrixserverlib.Invite && member.Domain == s.cfg.Matrix.ServerName {
			// localRoomMembers only adds joined members. An invite
			// should also be pushed to the target user.
			members = append(members, member)
		}
	}

	// TODO: run in parallel with localRoomMembers.
	roomName, err := s.roomName(ctx, event)
	if err != nil {
		return err
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
		if err := s.notifyLocal(ctx, event, mem, roomSize, roomName); err != nil {
			log.WithFields(log.Fields{
				"localpart": mem.Localpart,
			}).WithError(err).Debugf("Unable to push to local user")
			continue
		}
	}

	return nil
}

type localMembership struct {
	gomatrixserverlib.MemberContent
	UserID    string
	Localpart string
	Domain    gomatrixserverlib.ServerName
}

func newLocalMembership(event *gomatrixserverlib.ClientEvent) (*localMembership, error) {
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
	req := &rsapi.QueryMembershipsForRoomRequest{
		RoomID:     roomID,
		JoinedOnly: true,
	}
	var res rsapi.QueryMembershipsForRoomResponse

	// XXX: This could potentially race if the state for the event is not known yet
	// e.g. the event came over federation but we do not have the full state persisted.
	if err := s.rsAPI.QueryMembershipsForRoom(ctx, req, &res); err != nil {
		return nil, 0, err
	}

	var members []*localMembership
	var ntotal int
	for _, event := range res.JoinEvents {
		member, err := newLocalMembership(&event)
		if err != nil {
			log.WithError(err).Errorf("Parsing MemberContent")
			continue
		}
		if member.Membership != gomatrixserverlib.Join {
			continue
		}
		if member.Domain != s.cfg.Matrix.ServerName {
			continue
		}

		ntotal++
		members = append(members, member)
	}

	return members, ntotal, nil
}

// roomName returns the name in the event (if type==m.room.name), or
// looks it up in roomserver. If there is no name,
// m.room.canonical_alias is consulted. Returns an empty string if the
// room has no name.
func (s *OutputRoomEventConsumer) roomName(ctx context.Context, event *gomatrixserverlib.HeaderedEvent) (string, error) {
	if event.Type() == gomatrixserverlib.MRoomName {
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
		return "", err
	}

	if eventS := res.StateEvents[roomNameTuple]; eventS != nil {
		return unmarshalRoomName(eventS)
	}

	if event.Type() == gomatrixserverlib.MRoomCanonicalAlias {
		alias, err := unmarshalCanonicalAlias(event)
		if err != nil {
			return "", err
		}

		if alias != "" {
			return alias, nil
		}
	}

	if event := res.StateEvents[canonicalAliasTuple]; event != nil {
		return unmarshalCanonicalAlias(event)
	}

	return "", nil
}

var (
	canonicalAliasTuple = gomatrixserverlib.StateKeyTuple{EventType: gomatrixserverlib.MRoomCanonicalAlias}
	roomNameTuple       = gomatrixserverlib.StateKeyTuple{EventType: gomatrixserverlib.MRoomName}
)

func unmarshalRoomName(event *gomatrixserverlib.HeaderedEvent) (string, error) {
	var nc eventutil.NameContent
	if err := json.Unmarshal(event.Content(), &nc); err != nil {
		return "", fmt.Errorf("unmarshaling NameContent: %w", err)
	}

	return nc.Name, nil
}

func unmarshalCanonicalAlias(event *gomatrixserverlib.HeaderedEvent) (string, error) {
	var cac eventutil.CanonicalAliasContent
	if err := json.Unmarshal(event.Content(), &cac); err != nil {
		return "", fmt.Errorf("unmarshaling CanonicalAliasContent: %w", err)
	}

	return cac.Alias, nil
}

// notifyLocal finds the right push actions for a local user, given an event.
func (s *OutputRoomEventConsumer) notifyLocal(ctx context.Context, event *gomatrixserverlib.HeaderedEvent, mem *localMembership, roomSize int, roomName string) error {
	actions, err := s.evaluatePushRules(ctx, event, mem, roomSize)
	if err != nil {
		return err
	}
	a, tweaks, err := pushrules.ActionsToTweaks(actions)
	if err != nil {
		return err
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

	devicesByURLAndFormat, profileTag, err := s.localPushDevices(ctx, mem.Localpart, tweaks)
	if err != nil {
		return err
	}

	n := &api.Notification{
		Actions: actions,
		// UNSPEC: the spec doesn't say this is a ClientEvent, but the
		// fields seem to match. room_id should be missing, which
		// matches the behaviour of FormatSync.
		Event: gomatrixserverlib.HeaderedToClientEvent(event, gomatrixserverlib.FormatSync),
		// TODO: this is per-device, but it's not part of the primary
		// key. So inserting one notification per profile tag doesn't
		// make sense. What is this supposed to be? Sytests require it
		// to "work", but they only use a single device.
		ProfileTag: profileTag,
		RoomID:     event.RoomID(),
		TS:         gomatrixserverlib.AsTimestamp(time.Now()),
	}
	if err = s.db.InsertNotification(ctx, mem.Localpart, event.EventID(), tweaks, n); err != nil {
		return err
	}

	if err = s.syncProducer.GetAndSendNotificationData(ctx, mem.UserID, event.RoomID()); err != nil {
		return err
	}

	// We do this after InsertNotification. Thus, this should always return >=1.
	userNumUnreadNotifs, err := s.db.GetNotificationCount(ctx, mem.Localpart, tables.AllNotifications)
	if err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"event_id":   event.EventID(),
		"room_id":    event.RoomID(),
		"localpart":  mem.Localpart,
		"num_urls":   len(devicesByURLAndFormat),
		"num_unread": userNumUnreadNotifs,
	}).Tracef("Notifying single member")

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
			s.deleteRejectedPushers(ctx, rejected, mem.Localpart)
		}
	}()

	return nil
}

// evaluatePushRules fetches and evaluates the push rules of a local
// user. Returns actions (including dont_notify).
func (s *OutputRoomEventConsumer) evaluatePushRules(ctx context.Context, event *gomatrixserverlib.HeaderedEvent, mem *localMembership, roomSize int) ([]*pushrules.Action, error) {
	if event.Sender() == mem.UserID {
		// SPEC: Homeservers MUST NOT notify the Push Gateway for
		// events that the user has sent themselves.
		return nil, nil
	}

	var res api.QueryPushRulesResponse
	if err := s.userAPI.QueryPushRules(ctx, &api.QueryPushRulesRequest{UserID: mem.UserID}, &res); err != nil {
		return nil, err
	}

	ec := &ruleSetEvalContext{
		ctx:      ctx,
		rsAPI:    s.rsAPI,
		mem:      mem,
		roomID:   event.RoomID(),
		roomSize: roomSize,
	}
	eval := pushrules.NewRuleSetEvaluator(ec, &res.RuleSets.Global)
	rule, err := eval.MatchEvent(event.Event)
	if err != nil {
		return nil, err
	}
	if rule == nil {
		// SPEC: If no rules match an event, the homeserver MUST NOT
		// notify the Push Gateway for that event.
		return nil, err
	}

	log.WithFields(log.Fields{
		"event_id":  event.EventID(),
		"room_id":   event.RoomID(),
		"localpart": mem.Localpart,
		"rule_id":   rule.RuleID,
	}).Tracef("Matched a push rule")

	return rule.Actions, nil
}

type ruleSetEvalContext struct {
	ctx      context.Context
	rsAPI    rsapi.RoomserverInternalAPI
	mem      *localMembership
	roomID   string
	roomSize int
}

func (rse *ruleSetEvalContext) UserDisplayName() string { return rse.mem.DisplayName }

func (rse *ruleSetEvalContext) RoomMemberCount() (int, error) { return rse.roomSize, nil }

func (rse *ruleSetEvalContext) HasPowerLevel(userID, levelKey string) (bool, error) {
	req := &rsapi.QueryLatestEventsAndStateRequest{
		RoomID: rse.roomID,
		StateToFetch: []gomatrixserverlib.StateKeyTuple{
			{EventType: "m.room.power_levels"},
		},
	}
	var res rsapi.QueryLatestEventsAndStateResponse
	if err := rse.rsAPI.QueryLatestEventsAndState(rse.ctx, req, &res); err != nil {
		return false, err
	}
	for _, ev := range res.StateEvents {
		if ev.Type() != gomatrixserverlib.MRoomPowerLevels {
			continue
		}

		plc, err := gomatrixserverlib.NewPowerLevelContentFromEvent(ev.Event)
		if err != nil {
			return false, err
		}
		return plc.UserLevel(userID) >= plc.NotificationLevel(levelKey), nil
	}
	return true, nil
}

// localPushDevices pushes to the configured devices of a local
// user. The map keys are [url][format].
func (s *OutputRoomEventConsumer) localPushDevices(ctx context.Context, localpart string, tweaks map[string]interface{}) (map[string]map[string][]*pushgateway.Device, string, error) {
	pusherDevices, err := util.GetPushDevices(ctx, localpart, tweaks, s.db)
	if err != nil {
		return nil, "", err
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
func (s *OutputRoomEventConsumer) notifyHTTP(ctx context.Context, event *gomatrixserverlib.HeaderedEvent, url, format string, devices []*pushgateway.Device, localpart, roomName string, userNumUnreadNotifs int) ([]*pushgateway.Device, error) {
	var req pushgateway.NotifyRequest
	switch format {
	case "event_id_only":
		req = pushgateway.NotifyRequest{
			Notification: pushgateway.Notification{
				Counts:  &pushgateway.Counts{},
				Devices: devices,
				EventID: event.EventID(),
				RoomID:  event.RoomID(),
			},
		}

	default:
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
				Sender:   event.Sender(),
				Type:     event.Type(),
			},
		}
		if mem, err := event.Membership(); err == nil {
			req.Notification.Membership = mem
		}
		if event.StateKey() != nil && *event.StateKey() == fmt.Sprintf("@%s:%s", localpart, s.cfg.Matrix.ServerName) {
			req.Notification.UserIsTarget = true
		}
	}

	log.WithFields(log.Fields{
		"event_id":    event.EventID(),
		"url":         url,
		"localpart":   localpart,
		"app_id0":     devices[0].AppID,
		"pushkey":     devices[0].PushKey,
		"num_devices": len(devices),
	}).Debugf("Notifying HTTP push gateway")

	var res pushgateway.NotifyResponse
	if err := s.pgClient.Notify(ctx, url, &req, &res); err != nil {
		return nil, err
	}

	log.WithFields(log.Fields{
		"event_id":     event.EventID(),
		"url":          url,
		"localpart":    localpart,
		"num_rejected": len(res.Rejected),
	}).Tracef("HTTP push gateway result")

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
func (s *OutputRoomEventConsumer) deleteRejectedPushers(ctx context.Context, devices []*pushgateway.Device, localpart string) {
	log.WithFields(log.Fields{
		"localpart":   localpart,
		"app_id0":     devices[0].AppID,
		"num_devices": len(devices),
	}).Infof("Deleting pushers rejected by the HTTP push gateway")

	for _, d := range devices {
		if err := s.db.RemovePusher(ctx, d.AppID, d.PushKey, localpart); err != nil {
			log.WithFields(log.Fields{
				"localpart": localpart,
			}).WithError(err).Errorf("Unable to delete rejected pusher")
		}
	}
}
