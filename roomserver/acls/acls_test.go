// Copyright 2024 New Vector Ltd.
// Copyright 2020 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package acls

import (
	"context"
	"regexp"
	"testing"

	"github.com/element-hq/dendrite/roomserver/storage/tables"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/stretchr/testify/assert"
)

func TestOpenACLsWithBlacklist(t *testing.T) {
	roomID := "!test:test.com"
	allowRegex, err := compileACLRegex("*")
	if err != nil {
		t.Fatal(err)
	}
	denyRegex, err := compileACLRegex("foo.com")
	if err != nil {
		t.Fatal(err)
	}

	acls := ServerACLs{
		acls: make(map[string]*serverACL),
	}

	acls.acls[roomID] = &serverACL{
		ServerACL: ServerACL{
			AllowIPLiterals: true,
		},
		allowedRegexes: []**regexp.Regexp{&allowRegex},
		deniedRegexes:  []**regexp.Regexp{&denyRegex},
	}

	if acls.IsServerBannedFromRoom("1.2.3.4", roomID) {
		t.Fatal("Expected 1.2.3.4 to be allowed but wasn't")
	}
	if acls.IsServerBannedFromRoom("1.2.3.4:2345", roomID) {
		t.Fatal("Expected 1.2.3.4:2345 to be allowed but wasn't")
	}
	if !acls.IsServerBannedFromRoom("foo.com", roomID) {
		t.Fatal("Expected foo.com to be banned but wasn't")
	}
	if !acls.IsServerBannedFromRoom("foo.com:3456", roomID) {
		t.Fatal("Expected foo.com:3456 to be banned but wasn't")
	}
	if acls.IsServerBannedFromRoom("bar.com", roomID) {
		t.Fatal("Expected bar.com to be allowed but wasn't")
	}
	if acls.IsServerBannedFromRoom("bar.com:4567", roomID) {
		t.Fatal("Expected bar.com:4567 to be allowed but wasn't")
	}
}

func TestDefaultACLsWithWhitelist(t *testing.T) {
	roomID := "!test:test.com"
	allowRegex, err := compileACLRegex("foo.com")
	if err != nil {
		t.Fatal(err)
	}

	acls := ServerACLs{
		acls: make(map[string]*serverACL),
	}

	acls.acls[roomID] = &serverACL{
		ServerACL: ServerACL{
			AllowIPLiterals: false,
		},
		allowedRegexes: []**regexp.Regexp{&allowRegex},
		deniedRegexes:  []**regexp.Regexp{},
	}

	if !acls.IsServerBannedFromRoom("1.2.3.4", roomID) {
		t.Fatal("Expected 1.2.3.4 to be banned but wasn't")
	}
	if !acls.IsServerBannedFromRoom("1.2.3.4:2345", roomID) {
		t.Fatal("Expected 1.2.3.4:2345 to be banned but wasn't")
	}
	if acls.IsServerBannedFromRoom("foo.com", roomID) {
		t.Fatal("Expected foo.com to be allowed but wasn't")
	}
	if acls.IsServerBannedFromRoom("foo.com:3456", roomID) {
		t.Fatal("Expected foo.com:3456 to be allowed but wasn't")
	}
	if !acls.IsServerBannedFromRoom("bar.com", roomID) {
		t.Fatal("Expected bar.com to be allowed but wasn't")
	}
	if !acls.IsServerBannedFromRoom("baz.com", roomID) {
		t.Fatal("Expected baz.com to be allowed but wasn't")
	}
	if !acls.IsServerBannedFromRoom("qux.com:4567", roomID) {
		t.Fatal("Expected qux.com:4567 to be allowed but wasn't")
	}
}

var (
	content1 = `{"allow":["*"],"allow_ip_literals":false,"deny":["hello.world", "*.hello.world"]}`
)

type dummyACLDB struct{}

func (d dummyACLDB) RoomsWithACLs(ctx context.Context) ([]string, error) {
	return []string{"1", "2"}, nil
}

func (d dummyACLDB) GetBulkStateContent(ctx context.Context, roomIDs []string, tuples []gomatrixserverlib.StateKeyTuple, allowWildcards bool) ([]tables.StrippedEvent, error) {
	return []tables.StrippedEvent{
		{
			RoomID:       "1",
			ContentValue: content1,
		},
		{
			RoomID:       "2",
			ContentValue: content1,
		},
	}, nil
}

func TestCachedRegex(t *testing.T) {
	db := dummyACLDB{}
	wantBannedServer := spec.ServerName("hello.world")

	acls := NewServerACLs(db)

	// Check that hello.world is banned in room 1
	banned := acls.IsServerBannedFromRoom(wantBannedServer, "1")
	assert.True(t, banned)

	// Check that hello.world is banned in room 2
	banned = acls.IsServerBannedFromRoom(wantBannedServer, "2")
	assert.True(t, banned)

	// Check that matrix.hello.world is banned in room 2
	banned = acls.IsServerBannedFromRoom("matrix."+wantBannedServer, "2")
	assert.True(t, banned)
}
