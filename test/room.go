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
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/matrix-org/dendrite/internal/eventutil"
	"github.com/matrix-org/gomatrixserverlib"
)

type Preset int

var (
	PresetNone               Preset = 0
	PresetPrivateChat        Preset = 1
	PresetPublicChat         Preset = 2
	PresetTrustedPrivateChat Preset = 3

	roomIDCounter = int64(0)

	testKeyID      = gomatrixserverlib.KeyID("ed25519:test")
	testPrivateKey = ed25519.NewKeyFromSeed([]byte{
		1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16,
		17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32,
	})
)

type Room struct {
	ID      string
	Version gomatrixserverlib.RoomVersion
	preset  Preset
	creator *User

	authEvents gomatrixserverlib.AuthEvents
	events     []*gomatrixserverlib.HeaderedEvent
}

// Create a new test room. Automatically creates the initial create events.
func NewRoom(t *testing.T, creator *User, modifiers ...roomModifier) *Room {
	t.Helper()
	counter := atomic.AddInt64(&roomIDCounter, 1)

	// set defaults then let roomModifiers override
	r := &Room{
		ID:         fmt.Sprintf("!%d:localhost", counter),
		creator:    creator,
		authEvents: gomatrixserverlib.NewAuthEvents(nil),
		preset:     PresetPublicChat,
		Version:    gomatrixserverlib.RoomVersionV9,
	}
	for _, m := range modifiers {
		m(t, r)
	}
	r.insertCreateEvents(t)
	return r
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
		hisVis.HistoryVisibility = "shared"
	case PresetPublicChat:
		joinRule.JoinRule = "public"
		hisVis.HistoryVisibility = "shared"
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
		mod.privKey = testPrivateKey
	}
	if mod.keyID == "" {
		mod.keyID = testKeyID
	}
	if mod.originServerTS.IsZero() {
		mod.originServerTS = time.Now()
	}
	if mod.origin == "" {
		mod.origin = gomatrixserverlib.ServerName("localhost")
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
	return ev.Headered(r.Version)
}

// Add a new event to this room DAG. Not thread-safe.
func (r *Room) InsertEvent(t *testing.T, he *gomatrixserverlib.HeaderedEvent) {
	t.Helper()
	// Add the event to the list of auth events
	r.events = append(r.events, he)
	if he.StateKey() != nil {
		err := r.authEvents.AddEvent(he.Unwrap())
		if err != nil {
			t.Fatalf("InsertEvent: failed to add event to auth events: %s", err)
		}
	}
}

func (r *Room) Events() []*gomatrixserverlib.HeaderedEvent {
	return r.events
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

func RoomVersion(ver gomatrixserverlib.RoomVersion) roomModifier {
	return func(t *testing.T, r *Room) {
		r.Version = ver
	}
}
