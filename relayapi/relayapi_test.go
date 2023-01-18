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

package relayapi_test

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-yggdrasil/signing"
	"github.com/matrix-org/dendrite/relayapi"
	"github.com/matrix-org/dendrite/test"
	"github.com/matrix-org/dendrite/test/testrig"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/stretchr/testify/assert"
)

func TestCreateNewRelayInternalAPI(t *testing.T) {
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		base, close := testrig.CreateBaseDendrite(t, dbType)
		defer close()

		relayAPI := relayapi.NewRelayInternalAPI(base, nil, nil, nil, nil)
		assert.NotNil(t, relayAPI)
	})
}

func TestCreateRelayInternalInvalidDatabasePanics(t *testing.T) {
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		base, close := testrig.CreateBaseDendrite(t, dbType)
		if dbType == test.DBTypeSQLite {
			base.Cfg.RelayAPI.Database.ConnectionString = "file:"
		} else {
			base.Cfg.RelayAPI.Database.ConnectionString = "test"
		}
		defer close()

		assert.Panics(t, func() {
			relayapi.NewRelayInternalAPI(base, nil, nil, nil, nil)
		})
	})
}

func TestCreateInvalidRelayPublicRoutesPanics(t *testing.T) {
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		base, close := testrig.CreateBaseDendrite(t, dbType)
		defer close()

		assert.Panics(t, func() {
			relayapi.AddPublicRoutes(base, nil, nil)
		})
	})
}

func createGetRelayTxnHTTPRequest(serverName gomatrixserverlib.ServerName, userID string) *http.Request {
	_, sk, _ := ed25519.GenerateKey(nil)
	keyID := signing.KeyID
	pk := sk.Public().(ed25519.PublicKey)
	origin := gomatrixserverlib.ServerName(hex.EncodeToString(pk))
	req := gomatrixserverlib.NewFederationRequest("GET", origin, serverName, "/_matrix/federation/v1/relay_txn/"+userID)
	content := gomatrixserverlib.RelayEntry{EntryID: 0}
	req.SetContent(content)
	req.Sign(origin, gomatrixserverlib.KeyID(keyID), sk)
	httpreq, _ := req.HTTPRequest()
	vars := map[string]string{"userID": userID}
	httpreq = mux.SetURLVars(httpreq, vars)
	return httpreq
}

type sendRelayContent struct {
	PDUs []json.RawMessage       `json:"pdus"`
	EDUs []gomatrixserverlib.EDU `json:"edus"`
}

func createSendRelayTxnHTTPRequest(serverName gomatrixserverlib.ServerName, txnID string, userID string) *http.Request {
	_, sk, _ := ed25519.GenerateKey(nil)
	keyID := signing.KeyID
	pk := sk.Public().(ed25519.PublicKey)
	origin := gomatrixserverlib.ServerName(hex.EncodeToString(pk))
	req := gomatrixserverlib.NewFederationRequest("PUT", origin, serverName, "/_matrix/federation/v1/send_relay/"+txnID+"/"+userID)
	content := sendRelayContent{}
	req.SetContent(content)
	req.Sign(origin, gomatrixserverlib.KeyID(keyID), sk)
	httpreq, _ := req.HTTPRequest()
	vars := map[string]string{"userID": userID, "txnID": txnID}
	httpreq = mux.SetURLVars(httpreq, vars)
	return httpreq
}

func TestCreateRelayPublicRoutes(t *testing.T) {
	base, close := testrig.CreateBaseDendrite(t, test.DBTypeSQLite)
	defer close()

	relayAPI := relayapi.NewRelayInternalAPI(base, nil, nil, nil, nil)
	assert.NotNil(t, relayAPI)

	serverKeyAPI := &signing.YggdrasilKeys{}
	keyRing := serverKeyAPI.KeyRing()
	relayapi.AddPublicRoutes(base, keyRing, relayAPI)

	testCases := []struct {
		name            string
		req             *http.Request
		wantCode        int
		wantJoinedRooms []string
	}{
		{
			name:     "relay_txn invalid user id",
			req:      createGetRelayTxnHTTPRequest(base.Cfg.Global.ServerName, "user:local"),
			wantCode: 400,
		},
		{
			name:     "relay_txn valid user id",
			req:      createGetRelayTxnHTTPRequest(base.Cfg.Global.ServerName, "@user:local"),
			wantCode: 200,
		},
		{
			name:     "send_relay invalid user id",
			req:      createSendRelayTxnHTTPRequest(base.Cfg.Global.ServerName, "123", "user:local"),
			wantCode: 400,
		},
		{
			name:     "send_relay valid user id",
			req:      createSendRelayTxnHTTPRequest(base.Cfg.Global.ServerName, "123", "@user:local"),
			wantCode: 200,
		},
	}

	for _, tc := range testCases {
		w := httptest.NewRecorder()
		base.PublicFederationAPIMux.ServeHTTP(w, tc.req)
		if w.Code != tc.wantCode {
			t.Fatalf("%s: got HTTP %d want %d", tc.name, w.Code, tc.wantCode)
		}
	}
}
