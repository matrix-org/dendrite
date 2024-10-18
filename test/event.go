// Copyright 2024 New Vector Ltd.
// Copyright 2022 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package test

import (
	"bytes"
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/element-hq/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
)

type eventMods struct {
	originServerTS time.Time
	origin         spec.ServerName
	stateKey       *string
	unsigned       interface{}
	keyID          gomatrixserverlib.KeyID
	privKey        ed25519.PrivateKey
	authEvents     []string
}

type eventModifier func(e *eventMods)

func WithTimestamp(ts time.Time) eventModifier {
	return func(e *eventMods) {
		e.originServerTS = ts
	}
}

func WithStateKey(skey string) eventModifier {
	return func(e *eventMods) {
		e.stateKey = &skey
	}
}

func WithUnsigned(unsigned interface{}) eventModifier {
	return func(e *eventMods) {
		e.unsigned = unsigned
	}
}

func WithAuthIDs(evs []string) eventModifier {
	return func(e *eventMods) {
		e.authEvents = evs
	}
}

func WithKeyID(keyID gomatrixserverlib.KeyID) eventModifier {
	return func(e *eventMods) {
		e.keyID = keyID
	}
}

func WithPrivateKey(pkey ed25519.PrivateKey) eventModifier {
	return func(e *eventMods) {
		e.privKey = pkey
	}
}

func WithOrigin(origin spec.ServerName) eventModifier {
	return func(e *eventMods) {
		e.origin = origin
	}
}

// Reverse a list of events
func Reversed(in []*types.HeaderedEvent) []*types.HeaderedEvent {
	out := make([]*types.HeaderedEvent, len(in))
	for i := 0; i < len(in); i++ {
		out[i] = in[len(in)-i-1]
	}
	return out
}

func AssertEventIDsEqual(t *testing.T, gotEventIDs []string, wants []*types.HeaderedEvent) {
	t.Helper()
	if len(gotEventIDs) != len(wants) {
		t.Errorf("length mismatch: got %d events, want %d", len(gotEventIDs), len(wants))
		return
	}
	for i := range wants {
		w := wants[i].EventID()
		g := gotEventIDs[i]
		if w != g {
			t.Errorf("event at index %d mismatch:\ngot  %s\n\nwant %s", i, string(g), string(w))
		}
	}
}

func AssertEventsEqual(t *testing.T, gots, wants []*types.HeaderedEvent) {
	t.Helper()
	if len(gots) != len(wants) {
		t.Fatalf("length mismatch: got %d events, want %d", len(gots), len(wants))
	}
	for i := range wants {
		w := wants[i].JSON()
		g := gots[i].JSON()
		if !bytes.Equal(w, g) {
			t.Errorf("event at index %d mismatch:\ngot  %s\n\nwant %s", i, string(g), string(w))
		}
	}
}
