// Copyright 2024 New Vector Ltd.
// Copyright 2022 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package routing_test

import (
	"encoding/hex"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/element-hq/dendrite/cmd/dendrite-demo-yggdrasil/signing"
	fedAPI "github.com/element-hq/dendrite/federationapi"
	"github.com/element-hq/dendrite/federationapi/routing"
	"github.com/element-hq/dendrite/internal/caching"
	"github.com/element-hq/dendrite/internal/httputil"
	"github.com/element-hq/dendrite/internal/sqlutil"
	"github.com/element-hq/dendrite/setup/jetstream"
	"github.com/element-hq/dendrite/test"
	"github.com/element-hq/dendrite/test/testrig"
	"github.com/gorilla/mux"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/ed25519"
)

const (
	testOrigin = spec.ServerName("kaer.morhen")
)

type sendContent struct {
	PDUs []json.RawMessage       `json:"pdus"`
	EDUs []gomatrixserverlib.EDU `json:"edus"`
}

func TestHandleSend(t *testing.T) {
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		cfg, processCtx, close := testrig.CreateConfig(t, dbType)
		cm := sqlutil.NewConnectionManager(processCtx, cfg.Global.DatabaseOptions)
		routers := httputil.NewRouters()
		defer close()

		fedMux := mux.NewRouter().SkipClean(true).PathPrefix(httputil.PublicFederationPathPrefix).Subrouter().UseEncodedPath()
		natsInstance := jetstream.NATSInstance{}
		routers.Federation = fedMux
		cfg.FederationAPI.Matrix.SigningIdentity.ServerName = testOrigin
		cfg.FederationAPI.Matrix.Metrics.Enabled = false
		fedapi := fedAPI.NewInternalAPI(processCtx, cfg, cm, &natsInstance, nil, nil, nil, nil, true)
		serverKeyAPI := &signing.YggdrasilKeys{}
		keyRing := serverKeyAPI.KeyRing()

		routing.Setup(routers, cfg, nil, fedapi, keyRing, nil, nil, &cfg.MSCs, nil, caching.DisableMetrics)

		handler := fedMux.Get(routing.SendRouteName).GetHandler().ServeHTTP
		_, sk, _ := ed25519.GenerateKey(nil)
		keyID := signing.KeyID
		pk := sk.Public().(ed25519.PublicKey)
		serverName := spec.ServerName(hex.EncodeToString(pk))
		req := fclient.NewFederationRequest("PUT", serverName, testOrigin, "/send/1234")
		content := sendContent{}
		err := req.SetContent(content)
		if err != nil {
			t.Fatalf("Error: %s", err.Error())
		}
		req.Sign(serverName, gomatrixserverlib.KeyID(keyID), sk)
		httpReq, err := req.HTTPRequest()
		if err != nil {
			t.Fatalf("Error: %s", err.Error())
		}
		vars := map[string]string{"txnID": "1234"}
		w := httptest.NewRecorder()
		httpReq = mux.SetURLVars(httpReq, vars)
		handler(w, httpReq)

		res := w.Result()
		assert.Equal(t, 200, res.StatusCode)
	})
}
