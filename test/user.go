// Copyright 2024 New Vector Ltd.
// Copyright 2022 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package test

import (
	"crypto/ed25519"
	"fmt"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/element-hq/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
)

var (
	userIDCounter = int64(0)

	serverName = spec.ServerName("test")
	keyID      = gomatrixserverlib.KeyID("ed25519:test")
	privateKey = ed25519.NewKeyFromSeed([]byte{
		1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16,
		17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32,
	})

	// private keys that tests can use
	PrivateKeyA = ed25519.NewKeyFromSeed([]byte{
		1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16,
		17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 77,
	})
	PrivateKeyB = ed25519.NewKeyFromSeed([]byte{
		1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16,
		17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 66,
	})
)

type User struct {
	ID          string
	Localpart   string
	AccountType api.AccountType
	// key ID and private key of the server who has this user, if known.
	keyID   gomatrixserverlib.KeyID
	privKey ed25519.PrivateKey
	srvName spec.ServerName
}

type UserOpt func(*User)

func WithSigningServer(srvName spec.ServerName, keyID gomatrixserverlib.KeyID, privKey ed25519.PrivateKey) UserOpt {
	return func(u *User) {
		u.keyID = keyID
		u.privKey = privKey
		u.srvName = srvName
	}
}

func WithAccountType(accountType api.AccountType) UserOpt {
	return func(u *User) {
		u.AccountType = accountType
	}
}

func NewUser(t *testing.T, opts ...UserOpt) *User {
	counter := atomic.AddInt64(&userIDCounter, 1)
	var u User
	for _, opt := range opts {
		opt(&u)
	}
	if u.keyID == "" || u.srvName == "" || u.privKey == nil {
		t.Logf("NewUser: missing signing server credentials; using default.")
		WithSigningServer(serverName, keyID, privateKey)(&u)
	}
	u.ID = fmt.Sprintf("@%d:%s", counter, u.srvName)
	u.Localpart = strconv.Itoa(int(counter))
	t.Logf("NewUser: created user %s", u.ID)
	return &u
}
