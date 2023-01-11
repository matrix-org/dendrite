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

package routing_test

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-yggdrasil/signing"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/relayapi"
	"github.com/matrix-org/dendrite/relayapi/internal"
	"github.com/matrix-org/dendrite/relayapi/routing"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/test"
	"github.com/matrix-org/dendrite/test/testrig"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/stretchr/testify/assert"
)

type sendRelayContent struct {
	PDUs []json.RawMessage       `json:"pdus"`
	EDUs []gomatrixserverlib.EDU `json:"edus"`
}

func TestHandleSendRelay(t *testing.T) {
	base, close := testrig.CreateBaseDendrite(t, test.DBTypeSQLite)
	defer close()

	fedMux := mux.NewRouter().SkipClean(true).PathPrefix(httputil.PublicFederationPathPrefix).Subrouter().UseEncodedPath()
	cfg := config.FederationAPI{
		Matrix: &config.Global{
			SigningIdentity: gomatrixserverlib.SigningIdentity{
				ServerName: "relay",
			},
		},
	}
	relayAPI := relayapi.NewRelayInternalAPI(base, nil, nil, nil, nil)
	serverKeyAPI := &signing.YggdrasilKeys{}
	keyRing := serverKeyAPI.KeyRing()
	r, ok := relayAPI.(*internal.RelayInternalAPI)
	if !ok {
		panic("This is a programming error.")
	}
	routing.Setup(fedMux, &cfg, r, keyRing)

	handler := fedMux.Get(routing.SendRelayTransactionRouteName).GetHandler().ServeHTTP
	_, sk, _ := ed25519.GenerateKey(nil)
	keyID := signing.KeyID
	pk := sk.Public().(ed25519.PublicKey)
	serverName := gomatrixserverlib.ServerName(hex.EncodeToString(pk))
	req := gomatrixserverlib.NewFederationRequest("PUT", serverName, "remote", "/send_relay/1234/@user:local")
	content := sendRelayContent{}
	err := req.SetContent(content)
	if err != nil {
		t.Fatalf("Error: %s", err.Error())
	}
	req.Sign(serverName, gomatrixserverlib.KeyID(keyID), sk)
	httpReq, err := req.HTTPRequest()
	if err != nil {
		t.Fatalf("Error: %s", err.Error())
	}
	vars := map[string]string{"userID": "@user:local", "txnID": "1234"}
	w := httptest.NewRecorder()
	httpReq = mux.SetURLVars(httpReq, vars)
	handler(w, httpReq)

	res := w.Result()
	assert.Equal(t, 200, res.StatusCode)
}

func TestHandleSendRelayBadUserID(t *testing.T) {
	base, close := testrig.CreateBaseDendrite(t, test.DBTypeSQLite)
	defer close()

	fedMux := mux.NewRouter().SkipClean(true).PathPrefix(httputil.PublicFederationPathPrefix).Subrouter().UseEncodedPath()
	cfg := config.FederationAPI{
		Matrix: &config.Global{
			SigningIdentity: gomatrixserverlib.SigningIdentity{
				ServerName: "relay",
			},
		},
	}
	relayAPI := relayapi.NewRelayInternalAPI(base, nil, nil, nil, nil)
	serverKeyAPI := &signing.YggdrasilKeys{}
	keyRing := serverKeyAPI.KeyRing()
	r, ok := relayAPI.(*internal.RelayInternalAPI)
	if !ok {
		panic("This is a programming error.")
	}
	routing.Setup(fedMux, &cfg, r, keyRing)

	handler := fedMux.Get(routing.SendRelayTransactionRouteName).GetHandler().ServeHTTP
	_, sk, _ := ed25519.GenerateKey(nil)
	keyID := signing.KeyID
	pk := sk.Public().(ed25519.PublicKey)
	serverName := gomatrixserverlib.ServerName(hex.EncodeToString(pk))
	req := gomatrixserverlib.NewFederationRequest("PUT", serverName, "remote", "/send_relay/1234/user")
	content := sendRelayContent{}
	err := req.SetContent(content)
	if err != nil {
		t.Fatalf("Error: %s", err.Error())
	}
	req.Sign(serverName, gomatrixserverlib.KeyID(keyID), sk)
	httpReq, err := req.HTTPRequest()
	if err != nil {
		t.Fatalf("Error: %s", err.Error())
	}
	vars := map[string]string{"userID": "user", "txnID": "1234"}
	w := httptest.NewRecorder()
	httpReq = mux.SetURLVars(httpReq, vars)
	handler(w, httpReq)

	res := w.Result()
	assert.NotEqual(t, 200, res.StatusCode)
}

func TestHandleRelayTxn(t *testing.T) {
	base, close := testrig.CreateBaseDendrite(t, test.DBTypeSQLite)
	defer close()

	fedMux := mux.NewRouter().SkipClean(true).PathPrefix(httputil.PublicFederationPathPrefix).Subrouter().UseEncodedPath()
	cfg := config.FederationAPI{
		Matrix: &config.Global{
			SigningIdentity: gomatrixserverlib.SigningIdentity{
				ServerName: "relay",
			},
		},
	}
	relayAPI := relayapi.NewRelayInternalAPI(base, nil, nil, nil, nil)
	serverKeyAPI := &signing.YggdrasilKeys{}
	keyRing := serverKeyAPI.KeyRing()
	r, ok := relayAPI.(*internal.RelayInternalAPI)
	if !ok {
		panic("This is a programming error.")
	}
	routing.Setup(fedMux, &cfg, r, keyRing)

	handler := fedMux.Get(routing.GetRelayTransactionRouteName).GetHandler().ServeHTTP
	_, sk, _ := ed25519.GenerateKey(nil)
	keyID := signing.KeyID
	pk := sk.Public().(ed25519.PublicKey)
	serverName := gomatrixserverlib.ServerName(hex.EncodeToString(pk))
	req := gomatrixserverlib.NewFederationRequest("GET", serverName, "remote", "/relay_txn/@user:local")
	content := gomatrixserverlib.RelayEntry{EntryID: 0}
	err := req.SetContent(content)
	if err != nil {
		t.Fatalf("Error: %s", err.Error())
	}
	req.Sign(serverName, gomatrixserverlib.KeyID(keyID), sk)
	httpReq, err := req.HTTPRequest()
	if err != nil {
		t.Fatalf("Error: %s", err.Error())
	}
	vars := map[string]string{"userID": "@user:local"}
	w := httptest.NewRecorder()
	httpReq = mux.SetURLVars(httpReq, vars)
	handler(w, httpReq)

	res := w.Result()
	assert.Equal(t, 200, res.StatusCode)
}

func TestHandleRelayTxnBadUserID(t *testing.T) {
	base, close := testrig.CreateBaseDendrite(t, test.DBTypeSQLite)
	defer close()

	fedMux := mux.NewRouter().SkipClean(true).PathPrefix(httputil.PublicFederationPathPrefix).Subrouter().UseEncodedPath()
	cfg := config.FederationAPI{
		Matrix: &config.Global{
			SigningIdentity: gomatrixserverlib.SigningIdentity{
				ServerName: "relay",
			},
		},
	}
	relayAPI := relayapi.NewRelayInternalAPI(base, nil, nil, nil, nil)
	serverKeyAPI := &signing.YggdrasilKeys{}
	keyRing := serverKeyAPI.KeyRing()
	r, ok := relayAPI.(*internal.RelayInternalAPI)
	if !ok {
		panic("This is a programming error.")
	}
	routing.Setup(fedMux, &cfg, r, keyRing)

	handler := fedMux.Get(routing.GetRelayTransactionRouteName).GetHandler().ServeHTTP
	_, sk, _ := ed25519.GenerateKey(nil)
	keyID := signing.KeyID
	pk := sk.Public().(ed25519.PublicKey)
	serverName := gomatrixserverlib.ServerName(hex.EncodeToString(pk))
	req := gomatrixserverlib.NewFederationRequest("GET", serverName, "remote", "/relay_txn/user")
	content := gomatrixserverlib.RelayEntry{EntryID: 0}
	err := req.SetContent(content)
	if err != nil {
		t.Fatalf("Error: %s", err.Error())
	}
	req.Sign(serverName, gomatrixserverlib.KeyID(keyID), sk)
	httpReq, err := req.HTTPRequest()
	if err != nil {
		t.Fatalf("Error: %s", err.Error())
	}
	vars := map[string]string{"userID": "user"}
	w := httptest.NewRecorder()
	httpReq = mux.SetURLVars(httpReq, vars)
	handler(w, httpReq)

	res := w.Result()
	assert.NotEqual(t, 200, res.StatusCode)
}
