// Copyright 2024 New Vector Ltd.
// Copyright 2022 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package relayapi_test

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/element-hq/dendrite/cmd/dendrite-demo-yggdrasil/signing"
	"github.com/element-hq/dendrite/internal/caching"
	"github.com/element-hq/dendrite/internal/httputil"
	"github.com/element-hq/dendrite/internal/sqlutil"
	"github.com/element-hq/dendrite/relayapi"
	"github.com/element-hq/dendrite/test"
	"github.com/element-hq/dendrite/test/testrig"
	"github.com/gorilla/mux"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/stretchr/testify/assert"
)

func TestCreateNewRelayInternalAPI(t *testing.T) {
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		cfg, processCtx, close := testrig.CreateConfig(t, dbType)
		caches := caching.NewRistrettoCache(128*1024*1024, time.Hour, caching.DisableMetrics)
		defer close()
		cm := sqlutil.NewConnectionManager(processCtx, cfg.Global.DatabaseOptions)
		relayAPI := relayapi.NewRelayInternalAPI(cfg, cm, nil, nil, nil, nil, true, caches)
		assert.NotNil(t, relayAPI)
	})
}

func TestCreateRelayInternalInvalidDatabasePanics(t *testing.T) {
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		cfg, processCtx, close := testrig.CreateConfig(t, dbType)
		if dbType == test.DBTypeSQLite {
			cfg.RelayAPI.Database.ConnectionString = "file:"
		} else {
			cfg.RelayAPI.Database.ConnectionString = "test"
		}
		defer close()
		cm := sqlutil.NewConnectionManager(processCtx, cfg.Global.DatabaseOptions)
		assert.Panics(t, func() {
			relayapi.NewRelayInternalAPI(cfg, cm, nil, nil, nil, nil, true, nil)
		})
	})
}

func TestCreateInvalidRelayPublicRoutesPanics(t *testing.T) {
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		cfg, _, close := testrig.CreateConfig(t, dbType)
		defer close()
		routers := httputil.NewRouters()
		assert.Panics(t, func() {
			relayapi.AddPublicRoutes(routers, cfg, nil, nil)
		})
	})
}

func createGetRelayTxnHTTPRequest(serverName spec.ServerName, userID string) *http.Request {
	_, sk, _ := ed25519.GenerateKey(nil)
	keyID := signing.KeyID
	pk := sk.Public().(ed25519.PublicKey)
	origin := spec.ServerName(hex.EncodeToString(pk))
	req := fclient.NewFederationRequest("GET", origin, serverName, "/_matrix/federation/v1/relay_txn/"+userID)
	content := fclient.RelayEntry{EntryID: 0}
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

func createSendRelayTxnHTTPRequest(serverName spec.ServerName, txnID string, userID string) *http.Request {
	_, sk, _ := ed25519.GenerateKey(nil)
	keyID := signing.KeyID
	pk := sk.Public().(ed25519.PublicKey)
	origin := spec.ServerName(hex.EncodeToString(pk))
	req := fclient.NewFederationRequest("PUT", origin, serverName, "/_matrix/federation/v1/send_relay/"+txnID+"/"+userID)
	content := sendRelayContent{}
	req.SetContent(content)
	req.Sign(origin, gomatrixserverlib.KeyID(keyID), sk)
	httpreq, _ := req.HTTPRequest()
	vars := map[string]string{"userID": userID, "txnID": txnID}
	httpreq = mux.SetURLVars(httpreq, vars)
	return httpreq
}

func TestCreateRelayPublicRoutes(t *testing.T) {
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		cfg, processCtx, close := testrig.CreateConfig(t, dbType)
		defer close()
		routers := httputil.NewRouters()
		caches := caching.NewRistrettoCache(128*1024*1024, time.Hour, caching.DisableMetrics)

		cm := sqlutil.NewConnectionManager(processCtx, cfg.Global.DatabaseOptions)

		relayAPI := relayapi.NewRelayInternalAPI(cfg, cm, nil, nil, nil, nil, true, caches)
		assert.NotNil(t, relayAPI)

		serverKeyAPI := &signing.YggdrasilKeys{}
		keyRing := serverKeyAPI.KeyRing()
		relayapi.AddPublicRoutes(routers, cfg, keyRing, relayAPI)

		testCases := []struct {
			name     string
			req      *http.Request
			wantCode int
		}{
			{
				name:     "relay_txn invalid user id",
				req:      createGetRelayTxnHTTPRequest(cfg.Global.ServerName, "user:local"),
				wantCode: 400,
			},
			{
				name:     "relay_txn valid user id",
				req:      createGetRelayTxnHTTPRequest(cfg.Global.ServerName, "@user:local"),
				wantCode: 200,
			},
			{
				name:     "send_relay invalid user id",
				req:      createSendRelayTxnHTTPRequest(cfg.Global.ServerName, "123", "user:local"),
				wantCode: 400,
			},
			{
				name:     "send_relay valid user id",
				req:      createSendRelayTxnHTTPRequest(cfg.Global.ServerName, "123", "@user:local"),
				wantCode: 200,
			},
		}

		for _, tc := range testCases {
			w := httptest.NewRecorder()
			routers.Federation.ServeHTTP(w, tc.req)
			if w.Code != tc.wantCode {
				t.Fatalf("%s: got HTTP %d want %d", tc.name, w.Code, tc.wantCode)
			}
		}
	})
}

func TestDisableRelayPublicRoutes(t *testing.T) {
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		cfg, processCtx, close := testrig.CreateConfig(t, dbType)
		defer close()
		routers := httputil.NewRouters()
		caches := caching.NewRistrettoCache(128*1024*1024, time.Hour, caching.DisableMetrics)

		cm := sqlutil.NewConnectionManager(processCtx, cfg.Global.DatabaseOptions)

		relayAPI := relayapi.NewRelayInternalAPI(cfg, cm, nil, nil, nil, nil, false, caches)
		assert.NotNil(t, relayAPI)

		serverKeyAPI := &signing.YggdrasilKeys{}
		keyRing := serverKeyAPI.KeyRing()
		relayapi.AddPublicRoutes(routers, cfg, keyRing, relayAPI)

		testCases := []struct {
			name     string
			req      *http.Request
			wantCode int
		}{
			{
				name:     "relay_txn valid user id",
				req:      createGetRelayTxnHTTPRequest(cfg.Global.ServerName, "@user:local"),
				wantCode: 404,
			},
			{
				name:     "send_relay valid user id",
				req:      createSendRelayTxnHTTPRequest(cfg.Global.ServerName, "123", "@user:local"),
				wantCode: 404,
			},
		}

		for _, tc := range testCases {
			w := httptest.NewRecorder()
			routers.Federation.ServeHTTP(w, tc.req)
			if w.Code != tc.wantCode {
				t.Fatalf("%s: got HTTP %d want %d", tc.name, w.Code, tc.wantCode)
			}
		}
	})
}
