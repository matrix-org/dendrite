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
	"context"
	"encoding/hex"
	"io"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-yggdrasil/signing"
	fedAPI "github.com/matrix-org/dendrite/federationapi"
	fedclient "github.com/matrix-org/dendrite/federationapi/api"
	fedInternal "github.com/matrix-org/dendrite/federationapi/internal"
	"github.com/matrix-org/dendrite/federationapi/routing"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/test"
	"github.com/matrix-org/dendrite/test/testrig"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/ed25519"
)

type fakeFedClient struct {
	fedclient.FederationClient
}

func (f *fakeFedClient) LookupRoomAlias(ctx context.Context, origin, s gomatrixserverlib.ServerName, roomAlias string) (res gomatrixserverlib.RespDirectory, err error) {
	return
}

func TestHandleQueryDirectory(t *testing.T) {
	base, close := testrig.CreateBaseDendrite(t, test.DBTypeSQLite)
	defer close()

	fedMux := mux.NewRouter().SkipClean(true).PathPrefix(httputil.PublicFederationPathPrefix).Subrouter().UseEncodedPath()
	base.PublicFederationAPIMux = fedMux
	base.Cfg.FederationAPI.Matrix.SigningIdentity.ServerName = testOrigin
	base.Cfg.FederationAPI.Matrix.Metrics.Enabled = false
	fedClient := fakeFedClient{}
	serverKeyAPI := &signing.YggdrasilKeys{}
	keyRing := serverKeyAPI.KeyRing()
	fedapi := fedAPI.NewInternalAPI(base, &fedClient, nil, nil, keyRing, true)
	userapi := fakeUserAPI{}
	r, ok := fedapi.(*fedInternal.FederationInternalAPI)
	if !ok {
		panic("This is a programming error.")
	}
	routing.Setup(base, nil, r, keyRing, &fedClient, &userapi, nil, &base.Cfg.MSCs, nil, nil)

	handler := fedMux.Get(routing.QueryDirectoryRouteName).GetHandler().ServeHTTP
	_, sk, _ := ed25519.GenerateKey(nil)
	keyID := signing.KeyID
	pk := sk.Public().(ed25519.PublicKey)
	serverName := gomatrixserverlib.ServerName(hex.EncodeToString(pk))
	req := gomatrixserverlib.NewFederationRequest("GET", serverName, testOrigin, "/query/directory?room_alias="+url.QueryEscape("#room:server"))
	type queryContent struct{}
	content := queryContent{}
	err := req.SetContent(content)
	if err != nil {
		t.Fatalf("Error: %s", err.Error())
	}
	req.Sign(serverName, gomatrixserverlib.KeyID(keyID), sk)
	httpReq, err := req.HTTPRequest()
	if err != nil {
		t.Fatalf("Error: %s", err.Error())
	}
	// vars := map[string]string{"room_alias": "#room:server"}
	w := httptest.NewRecorder()
	// httpReq = mux.SetURLVars(httpReq, vars)
	handler(w, httpReq)

	res := w.Result()
	data, _ := io.ReadAll(res.Body)
	println(string(data))
	assert.Equal(t, 200, res.StatusCode)
}
