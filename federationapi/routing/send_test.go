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
	"encoding/hex"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-yggdrasil/signing"
	fedAPI "github.com/matrix-org/dendrite/federationapi"
	fedInternal "github.com/matrix-org/dendrite/federationapi/internal"
	"github.com/matrix-org/dendrite/federationapi/routing"
	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/test"
	"github.com/matrix-org/dendrite/test/testrig"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/ed25519"
)

const (
	testOrigin = gomatrixserverlib.ServerName("kaer.morhen")
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
		r, ok := fedapi.(*fedInternal.FederationInternalAPI)
		if !ok {
			panic("This is a programming error.")
		}
		routing.Setup(routers, cfg, nil, r, keyRing, nil, nil, &cfg.MSCs, nil, nil, caching.DisableMetrics)

		handler := fedMux.Get(routing.SendRouteName).GetHandler().ServeHTTP
		_, sk, _ := ed25519.GenerateKey(nil)
		keyID := signing.KeyID
		pk := sk.Public().(ed25519.PublicKey)
		serverName := gomatrixserverlib.ServerName(hex.EncodeToString(pk))
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
