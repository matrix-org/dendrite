/* Copyright 2024 New Vector Ltd.
 * Copyright 2017 Vector Creations Ltd
 *
 * SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
 * Please see LICENSE files in the repository root for full details.
 */

package synctypes

import (
	"encoding/json"
	"fmt"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// PrevEventRef represents a reference to a previous event in a state event upgrade
type PrevEventRef struct {
	PrevContent   json.RawMessage `json:"prev_content"`
	ReplacesState string          `json:"replaces_state"`
	PrevSenderID  string          `json:"prev_sender"`
}

type ClientEventFormat int

const (
	// FormatAll will include all client event keys
	FormatAll ClientEventFormat = iota
	// FormatSync will include only the event keys required by the /sync API. Notably, this
	// means the 'room_id' will be missing from the events.
	FormatSync
	// FormatSyncFederation will include all event keys normally included in federated events.
	// This allows clients to request federated formatted events via the /sync API.
	FormatSyncFederation
)

// ClientFederationFields extends a ClientEvent to contain the additional fields present in a
// federation event. Used when the client requests `event_format` of type `federation`.
type ClientFederationFields struct {
	Depth      int64        `json:"depth,omitempty"`
	PrevEvents []string     `json:"prev_events,omitempty"`
	AuthEvents []string     `json:"auth_events,omitempty"`
	Signatures spec.RawJSON `json:"signatures,omitempty"`
	Hashes     spec.RawJSON `json:"hashes,omitempty"`
}

// ClientEvent is an event which is fit for consumption by clients, in accordance with the specification.
type ClientEvent struct {
	Content        spec.RawJSON   `json:"content"`
	EventID        string         `json:"event_id,omitempty"`         // EventID is omitted on receipt events
	OriginServerTS spec.Timestamp `json:"origin_server_ts,omitempty"` // OriginServerTS is omitted on receipt events
	RoomID         string         `json:"room_id,omitempty"`          // RoomID is omitted on /sync responses
	Sender         string         `json:"sender,omitempty"`           // Sender is omitted on receipt events
	SenderKey      spec.SenderID  `json:"sender_key,omitempty"`       // The SenderKey for events in pseudo ID rooms
	StateKey       *string        `json:"state_key,omitempty"`
	Type           string         `json:"type"`
	Unsigned       spec.RawJSON   `json:"unsigned,omitempty"`
	Redacts        string         `json:"redacts,omitempty"`

	// Only sent to clients when `event_format` == `federation`.
	ClientFederationFields
}

// ToClientEvents converts server events to client events.
func ToClientEvents(serverEvs []gomatrixserverlib.PDU, format ClientEventFormat, userIDForSender spec.UserIDForSender) []ClientEvent {
	evs := make([]ClientEvent, 0, len(serverEvs))
	for _, se := range serverEvs {
		if se == nil {
			continue // TODO: shouldn't happen?
		}
		ev, err := ToClientEvent(se, format, userIDForSender)
		if err != nil {
			logrus.WithError(err).Warn("Failed converting event to ClientEvent")
			continue
		}
		evs = append(evs, *ev)
	}
	return evs
}

// ToClientEventDefault converts a single server event to a client event.
// It provides default logic for event.SenderID & event.StateKey -> userID conversions.
func ToClientEventDefault(userIDQuery spec.UserIDForSender, event gomatrixserverlib.PDU) ClientEvent {
	ev, err := ToClientEvent(event, FormatAll, userIDQuery)
	if err != nil {
		return ClientEvent{}
	}
	return *ev
}

// If provided state key is a user ID (state keys beginning with @ are reserved for this purpose)
// fetch it's associated sender ID and use that instead. Otherwise returns the same state key back.
//
// # This function either returns the state key that should be used, or an error
//
// TODO: handle failure cases better (e.g. no sender ID)
func FromClientStateKey(roomID spec.RoomID, stateKey string, senderIDQuery spec.SenderIDForUser) (*string, error) {
	if len(stateKey) >= 1 && stateKey[0] == '@' {
		parsedStateKey, err := spec.NewUserID(stateKey, true)
		if err != nil {
			// If invalid user ID, then there is no associated state event.
			return nil, fmt.Errorf("Provided state key begins with @ but is not a valid user ID: %w", err)
		}
		senderID, err := senderIDQuery(roomID, *parsedStateKey)
		if err != nil {
			return nil, fmt.Errorf("Failed to query sender ID: %w", err)
		}
		if senderID == nil {
			// If no sender ID, then there is no associated state event.
			return nil, fmt.Errorf("No associated sender ID found.")
		}
		newStateKey := string(*senderID)
		return &newStateKey, nil
	} else {
		return &stateKey, nil
	}
}

// ToClientEvent converts a single server event to a client event.
func ToClientEvent(se gomatrixserverlib.PDU, format ClientEventFormat, userIDForSender spec.UserIDForSender) (*ClientEvent, error) {
	ce := ClientEvent{
		Content:        se.Content(),
		Sender:         string(se.SenderID()),
		Type:           se.Type(),
		StateKey:       se.StateKey(),
		Unsigned:       se.Unsigned(),
		OriginServerTS: se.OriginServerTS(),
		EventID:        se.EventID(),
		Redacts:        se.Redacts(),
	}

	switch format {
	case FormatAll:
		ce.RoomID = se.RoomID().String()
	case FormatSync:
	case FormatSyncFederation:
		ce.RoomID = se.RoomID().String()
		ce.AuthEvents = se.AuthEventIDs()
		ce.PrevEvents = se.PrevEventIDs()
		ce.Depth = se.Depth()
		// TODO: Set Signatures & Hashes fields
	}

	if format != FormatSyncFederation && se.Version() == gomatrixserverlib.RoomVersionPseudoIDs {
		err := updatePseudoIDs(&ce, se, userIDForSender, format)
		if err != nil {
			return nil, err
		}
	}

	return &ce, nil
}

func updatePseudoIDs(ce *ClientEvent, se gomatrixserverlib.PDU, userIDForSender spec.UserIDForSender, format ClientEventFormat) error {
	ce.SenderKey = se.SenderID()

	userID, err := userIDForSender(se.RoomID(), se.SenderID())
	if err == nil && userID != nil {
		ce.Sender = userID.String()
	}

	sk := se.StateKey()
	if sk != nil && *sk != "" {
		skUserID, err := userIDForSender(se.RoomID(), spec.SenderID(*sk))
		if err == nil && skUserID != nil {
			skString := skUserID.String()
			ce.StateKey = &skString
		}
	}

	var prev PrevEventRef
	if err := json.Unmarshal(se.Unsigned(), &prev); err == nil && prev.PrevSenderID != "" {
		prevUserID, err := userIDForSender(se.RoomID(), spec.SenderID(prev.PrevSenderID))
		if err == nil && userID != nil {
			prev.PrevSenderID = prevUserID.String()
		} else {
			errString := "userID unknown"
			if err != nil {
				errString = err.Error()
			}
			logrus.Warnf("Failed to find userID for prev_sender in ClientEvent: %s", errString)
			// NOTE: Not much can be done here, so leave the previous value in place.
		}
		ce.Unsigned, err = json.Marshal(prev)
		if err != nil {
			err = fmt.Errorf("Failed to marshal unsigned content for ClientEvent: %w", err)
			return err
		}
	}

	switch se.Type() {
	case spec.MRoomCreate:
		updatedContent, err := updateCreateEvent(se.Content(), userIDForSender, se.RoomID())
		if err != nil {
			err = fmt.Errorf("Failed to update m.room.create event for ClientEvent: %w", err)
			return err
		}
		ce.Content = updatedContent
	case spec.MRoomMember:
		updatedEvent, err := updateInviteEvent(userIDForSender, se, format)
		if err != nil {
			err = fmt.Errorf("Failed to update m.room.member event for ClientEvent: %w", err)
			return err
		}
		if updatedEvent != nil {
			ce.Unsigned = updatedEvent.Unsigned()
		}
	case spec.MRoomPowerLevels:
		updatedEvent, err := updatePowerLevelEvent(userIDForSender, se, format)
		if err != nil {
			err = fmt.Errorf("Failed update m.room.power_levels event for ClientEvent: %w", err)
			return err
		}
		if updatedEvent != nil {
			ce.Content = updatedEvent.Content()
			ce.Unsigned = updatedEvent.Unsigned()
		}
	}

	return nil
}

func updateCreateEvent(content spec.RawJSON, userIDForSender spec.UserIDForSender, roomID spec.RoomID) (spec.RawJSON, error) {
	if creator := gjson.GetBytes(content, "creator"); creator.Exists() {
		oldCreator := creator.Str
		userID, err := userIDForSender(roomID, spec.SenderID(oldCreator))
		if err != nil {
			err = fmt.Errorf("Failed to find userID for creator in ClientEvent: %w", err)
			return nil, err
		}

		if userID != nil {
			var newCreatorBytes, newContent []byte
			newCreatorBytes, err = json.Marshal(userID.String())
			if err != nil {
				err = fmt.Errorf("Failed to marshal new creator for ClientEvent: %w", err)
				return nil, err
			}

			newContent, err = sjson.SetRawBytes([]byte(content), "creator", newCreatorBytes)
			if err != nil {
				err = fmt.Errorf("Failed to set new creator for ClientEvent: %w", err)
				return nil, err
			}

			return newContent, nil
		}
	}

	return content, nil
}

func updateInviteEvent(userIDForSender spec.UserIDForSender, ev gomatrixserverlib.PDU, eventFormat ClientEventFormat) (gomatrixserverlib.PDU, error) {
	if inviteRoomState := gjson.GetBytes(ev.Unsigned(), "invite_room_state"); inviteRoomState.Exists() {
		userID, err := userIDForSender(ev.RoomID(), ev.SenderID())
		if err != nil || userID == nil {
			if err != nil {
				err = fmt.Errorf("invalid userID found when updating invite_room_state: %w", err)
			}
			return nil, err
		}

		newState, err := GetUpdatedInviteRoomState(userIDForSender, inviteRoomState, ev, ev.RoomID(), eventFormat)
		if err != nil {
			return nil, err
		}

		var newEv []byte
		newEv, err = sjson.SetRawBytes(ev.JSON(), "unsigned.invite_room_state", newState)
		if err != nil {
			return nil, err
		}

		return gomatrixserverlib.MustGetRoomVersion(ev.Version()).NewEventFromTrustedJSON(newEv, false)
	}

	return ev, nil
}

type InviteRoomStateEvent struct {
	Content  spec.RawJSON `json:"content"`
	SenderID string       `json:"sender"`
	StateKey *string      `json:"state_key"`
	Type     string       `json:"type"`
}

func GetUpdatedInviteRoomState(userIDForSender spec.UserIDForSender, inviteRoomState gjson.Result, event gomatrixserverlib.PDU, roomID spec.RoomID, eventFormat ClientEventFormat) (spec.RawJSON, error) {
	var res spec.RawJSON
	inviteStateEvents := []InviteRoomStateEvent{}
	err := json.Unmarshal([]byte(inviteRoomState.Raw), &inviteStateEvents)
	if err != nil {
		return nil, err
	}

	if event.Version() == gomatrixserverlib.RoomVersionPseudoIDs && eventFormat != FormatSyncFederation {
		for i, ev := range inviteStateEvents {
			userID, userIDErr := userIDForSender(roomID, spec.SenderID(ev.SenderID))
			if userIDErr != nil {
				return nil, userIDErr
			}
			if userID != nil {
				inviteStateEvents[i].SenderID = userID.String()
			}

			if ev.StateKey != nil && *ev.StateKey != "" {
				userID, senderErr := userIDForSender(roomID, spec.SenderID(*ev.StateKey))
				if senderErr != nil {
					return nil, senderErr
				}
				if userID != nil {
					user := userID.String()
					inviteStateEvents[i].StateKey = &user
				}
			}

			updatedContent, updateErr := updateCreateEvent(ev.Content, userIDForSender, roomID)
			if updateErr != nil {
				updateErr = fmt.Errorf("Failed to update m.room.create event for ClientEvent: %w", userIDErr)
				return nil, updateErr
			}
			inviteStateEvents[i].Content = updatedContent
		}
	}

	res, err = json.Marshal(inviteStateEvents)
	if err != nil {
		return nil, err
	}

	return res, nil
}

func updatePowerLevelEvent(userIDForSender spec.UserIDForSender, se gomatrixserverlib.PDU, eventFormat ClientEventFormat) (gomatrixserverlib.PDU, error) {
	if !se.StateKeyEquals("") {
		return se, nil
	}

	newEv := se.JSON()

	usersField := gjson.GetBytes(se.JSON(), "content.users")
	if usersField.Exists() {
		pls, err := gomatrixserverlib.NewPowerLevelContentFromEvent(se)
		if err != nil {
			return nil, err
		}

		newPls := make(map[string]int64)
		var userID *spec.UserID
		for user, level := range pls.Users {
			if eventFormat != FormatSyncFederation {
				userID, err = userIDForSender(se.RoomID(), spec.SenderID(user))
				if err != nil {
					return nil, err
				}
				user = userID.String()
			}
			newPls[user] = level
		}

		var newPlBytes []byte
		newPlBytes, err = json.Marshal(newPls)
		if err != nil {
			return nil, err
		}
		newEv, err = sjson.SetRawBytes(se.JSON(), "content.users", newPlBytes)
		if err != nil {
			return nil, err
		}
	}

	// do the same for prev content
	prevUsersField := gjson.GetBytes(se.JSON(), "unsigned.prev_content.users")
	if prevUsersField.Exists() {
		prevContent := gjson.GetBytes(se.JSON(), "unsigned.prev_content")
		if !prevContent.Exists() {
			evNew, err := gomatrixserverlib.MustGetRoomVersion(se.Version()).NewEventFromTrustedJSON(newEv, false)
			if err != nil {
				return nil, err
			}

			return evNew, err
		}
		pls := gomatrixserverlib.PowerLevelContent{}
		err := json.Unmarshal([]byte(prevContent.Raw), &pls)
		if err != nil {
			return nil, err
		}

		newPls := make(map[string]int64)
		for user, level := range pls.Users {
			if eventFormat != FormatSyncFederation {
				userID, userErr := userIDForSender(se.RoomID(), spec.SenderID(user))
				if userErr != nil {
					return nil, userErr
				}
				user = userID.String()
			}
			newPls[user] = level
		}

		var newPlBytes []byte
		newPlBytes, err = json.Marshal(newPls)
		if err != nil {
			return nil, err
		}
		newEv, err = sjson.SetRawBytes(newEv, "unsigned.prev_content.users", newPlBytes)
		if err != nil {
			return nil, err
		}
	}

	evNew, err := gomatrixserverlib.MustGetRoomVersion(se.Version()).NewEventFromTrustedJSONWithEventID(se.EventID(), newEv, false)
	if err != nil {
		return nil, err
	}

	return evNew, err
}
