// Copyright 2022 The Matrix.org Foundation C.I.C.
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

package test

import (
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/matrix-org/gomatrixserverlib"

	"github.com/matrix-org/dendrite/internal/eventutil"
)

type Preset int

var (
	PresetNone               Preset = 0
	PresetPrivateChat        Preset = 1
	PresetPublicChat         Preset = 2
	PresetTrustedPrivateChat Preset = 3

	roomIDCounter = int64(0)
)

type Room struct {
	ID         string
	Version    gomatrixserverlib.RoomVersion
	preset     Preset
	visibility gomatrixserverlib.HistoryVisibility
	creator    *User

	authEvents   gomatrixserverlib.AuthEvents
	currentState map[string]*gomatrixserverlib.HeaderedEvent
	events       []*gomatrixserverlib.HeaderedEvent
}

// Create a new test room. Automatically creates the initial create events.
func NewRoom(t *testing.T, creator *User, modifiers ...roomModifier) *Room {
	t.Helper()
	counter := atomic.AddInt64(&roomIDCounter, 1)
	if creator.srvName == "" {
		t.Fatalf("NewRoom: creator doesn't belong to a server: %+v", *creator)
	}
	r := &Room{
		ID:           fmt.Sprintf("!%d:%s", counter, creator.srvName),
		creator:      creator,
		authEvents:   gomatrixserverlib.NewAuthEvents(nil),
		preset:       PresetPublicChat,
		Version:      gomatrixserverlib.RoomVersionV9,
		currentState: make(map[string]*gomatrixserverlib.HeaderedEvent),
		visibility:   gomatrixserverlib.HistoryVisibilityShared,
	}
	for _, m := range modifiers {
		m(t, r)
	}
	r.insertCreateEvents(t)
	return r
}

func (r *Room) MustGetAuthEventRefsForEvent(t *testing.T, needed gomatrixserverlib.StateNeeded) []gomatrixserverlib.EventReference {
	t.Helper()
	a, err := needed.AuthEventReferences(&r.authEvents)
	if err != nil {
		t.Fatalf("MustGetAuthEvents: %v", err)
	}
	return a
}

func (r *Room) ForwardExtremities() []string {
	if len(r.events) == 0 {
		return nil
	}
	return []string{
		r.events[len(r.events)-1].EventID(),
	}
}

func (r *Room) insertCreateEvents(t *testing.T) {
	t.Helper()
	var joinRule gomatrixserverlib.JoinRuleContent
	var hisVis gomatrixserverlib.HistoryVisibilityContent
	plContent := eventutil.InitialPowerLevelsContent(r.creator.ID)
	switch r.preset {
	case PresetTrustedPrivateChat:
		fallthrough
	case PresetPrivateChat:
		joinRule.JoinRule = "invite"
		hisVis.HistoryVisibility = gomatrixserverlib.HistoryVisibilityShared
	case PresetPublicChat:
		joinRule.JoinRule = "public"
		hisVis.HistoryVisibility = gomatrixserverlib.HistoryVisibilityShared
	}

	if r.visibility != "" {
		hisVis.HistoryVisibility = r.visibility
	}

	r.CreateAndInsert(t, r.creator, gomatrixserverlib.MRoomCreate, map[string]interface{}{
		"creator":      r.creator.ID,
		"room_version": r.Version,
	}, WithStateKey(""))
	r.CreateAndInsert(t, r.creator, gomatrixserverlib.MRoomMember, map[string]interface{}{
		"membership": "join",
	}, WithStateKey(r.creator.ID))
	r.CreateAndInsert(t, r.creator, gomatrixserverlib.MRoomPowerLevels, plContent, WithStateKey(""))
	r.CreateAndInsert(t, r.creator, gomatrixserverlib.MRoomJoinRules, joinRule, WithStateKey(""))
	r.CreateAndInsert(t, r.creator, gomatrixserverlib.MRoomHistoryVisibility, hisVis, WithStateKey(""))
}

// Create an event in this room but do not insert it. Does not modify the room in any way (depth, fwd extremities, etc) so is thread-safe.
func (r *Room) CreateEvent(t *testing.T, creator *User, eventType string, content interface{}, mods ...eventModifier) *gomatrixserverlib.HeaderedEvent {
	t.Helper()
	depth := 1 + len(r.events) // depth starts at 1

	// possible event modifiers (optional fields)
	mod := &eventMods{}
	for _, m := range mods {
		m(mod)
	}

	if mod.privKey == nil {
		mod.privKey = creator.privKey
	}
	if mod.keyID == "" {
		mod.keyID = creator.keyID
	}
	if mod.originServerTS.IsZero() {
		mod.originServerTS = time.Now()
	}
	if mod.origin == "" {
		mod.origin = creator.srvName
	}

	var unsigned gomatrixserverlib.RawJSON
	var err error
	if mod.unsigned != nil {
		unsigned, err = json.Marshal(mod.unsigned)
		if err != nil {
			t.Fatalf("CreateEvent[%s]: failed to marshal unsigned field: %s", eventType, err)
		}
	}

	builder := &gomatrixserverlib.EventBuilder{
		Sender:   creator.ID,
		RoomID:   r.ID,
		Type:     eventType,
		StateKey: mod.stateKey,
		Depth:    int64(depth),
		Unsigned: unsigned,
	}
	err = builder.SetContent(content)
	if err != nil {
		t.Fatalf("CreateEvent[%s]: failed to SetContent: %s", eventType, err)
	}
	if depth > 1 {
		builder.PrevEvents = []gomatrixserverlib.EventReference{r.events[len(r.events)-1].EventReference()}
	}

	eventsNeeded, err := gomatrixserverlib.StateNeededForEventBuilder(builder)
	if err != nil {
		t.Fatalf("CreateEvent[%s]: failed to StateNeededForEventBuilder: %s", eventType, err)
	}

	refs, err := eventsNeeded.AuthEventReferences(&r.authEvents)
	if err != nil {
		t.Fatalf("CreateEvent[%s]: failed to AuthEventReferences: %s", eventType, err)
	}
	builder.AuthEvents = refs

	if len(mod.authEvents) > 0 {
		builder.AuthEvents = mod.authEvents
	}

	ev, err := builder.Build(
		mod.originServerTS, mod.origin, mod.keyID,
		mod.privKey, r.Version,
	)
	if err != nil {
		t.Fatalf("CreateEvent[%s]: failed to build event: %s", eventType, err)
	}
	if err = gomatrixserverlib.Allowed(ev, &r.authEvents); err != nil {
		t.Fatalf("CreateEvent[%s]: failed to verify event was allowed: %s", eventType, err)
	}
	headeredEvent := ev.Headered(r.Version)
	headeredEvent.Visibility = r.visibility
	return headeredEvent
}

// Add a new event to this room DAG. Not thread-safe.
func (r *Room) InsertEvent(t *testing.T, he *gomatrixserverlib.HeaderedEvent) {
	t.Helper()
	// Add the event to the list of auth/state events
	r.events = append(r.events, he)
	if he.StateKey() != nil {
		err := r.authEvents.AddEvent(he.Unwrap())
		if err != nil {
			t.Fatalf("InsertEvent: failed to add event to auth events: %s", err)
		}
		r.currentState[he.Type()+" "+*he.StateKey()] = he
	}
}

func (r *Room) Events() []*gomatrixserverlib.HeaderedEvent {
	return r.events
}

func (r *Room) CurrentState() []*gomatrixserverlib.HeaderedEvent {
	events := make([]*gomatrixserverlib.HeaderedEvent, len(r.currentState))
	i := 0
	for _, e := range r.currentState {
		events[i] = e
		i++
	}
	return events
}

func (r *Room) CreateAndInsert(t *testing.T, creator *User, eventType string, content interface{}, mods ...eventModifier) *gomatrixserverlib.HeaderedEvent {
	t.Helper()
	he := r.CreateEvent(t, creator, eventType, content, mods...)
	r.InsertEvent(t, he)
	return he
}

// All room modifiers are below

type roomModifier func(t *testing.T, r *Room)

func RoomPreset(p Preset) roomModifier {
	return func(t *testing.T, r *Room) {
		switch p {
		case PresetPrivateChat:
			fallthrough
		case PresetPublicChat:
			fallthrough
		case PresetTrustedPrivateChat:
			fallthrough
		case PresetNone:
			r.preset = p
		default:
			t.Errorf("invalid RoomPreset: %v", p)
		}
	}
}

func RoomHistoryVisibility(vis gomatrixserverlib.HistoryVisibility) roomModifier {
	return func(t *testing.T, r *Room) {
		r.visibility = vis
	}
}

func RoomVersion(ver gomatrixserverlib.RoomVersion) roomModifier {
	return func(t *testing.T, r *Room) {
		r.Version = ver
	}
}
