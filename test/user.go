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
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
)

var (
	userIDCounter = int64(0)

	serverName = gomatrixserverlib.ServerName("test")
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
	accountType api.AccountType
	// key ID and private key of the server who has this user, if known.
	keyID   gomatrixserverlib.KeyID
	privKey ed25519.PrivateKey
	srvName gomatrixserverlib.ServerName
}

type UserOpt func(*User)

func WithSigningServer(srvName gomatrixserverlib.ServerName, keyID gomatrixserverlib.KeyID, privKey ed25519.PrivateKey) UserOpt {
	return func(u *User) {
		u.keyID = keyID
		u.privKey = privKey
		u.srvName = srvName
	}
}

func WithAccountType(accountType api.AccountType) UserOpt {
	return func(u *User) {
		u.accountType = accountType
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
	t.Logf("NewUser: created user %s", u.ID)
	return &u
}
