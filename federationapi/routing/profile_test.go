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
	fedInternal "github.com/matrix-org/dendrite/federationapi/internal"
	"github.com/matrix-org/dendrite/federationapi/routing"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/test"
	"github.com/matrix-org/dendrite/test/testrig"
	userAPI "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/ed25519"
)

type fakeUserAPI struct {
	userAPI.FederationUserAPI
}

func (u *fakeUserAPI) QueryProfile(ctx context.Context, req *userAPI.QueryProfileRequest, res *userAPI.QueryProfileResponse) error {
	return nil
}

func TestHandleQueryProfile(t *testing.T) {
	base, close := testrig.CreateBaseDendrite(t, test.DBTypeSQLite)
	defer close()

	defer func() {
		prometheus.Unregister(internal.PDUCountTotal)
		prometheus.Unregister(internal.EDUCountTotal)
	}()

	fedMux := mux.NewRouter().SkipClean(true).PathPrefix(httputil.PublicFederationPathPrefix).Subrouter().UseEncodedPath()
	keyMux := mux.NewRouter().SkipClean(true).PathPrefix(httputil.PublicKeyPathPrefix).Subrouter().UseEncodedPath()
	cfg := config.FederationAPI{
		Matrix: &config.Global{
			SigningIdentity: gomatrixserverlib.SigningIdentity{
				ServerName: "remote",
			},
		},
	}
	fedClient := fakeFedClient{}
	fedapi := fedAPI.NewInternalAPI(base, &fedClient, nil, nil, nil, true)
	serverKeyAPI := &signing.YggdrasilKeys{}
	keyRing := serverKeyAPI.KeyRing()
	userapi := fakeUserAPI{}
	r, ok := fedapi.(*fedInternal.FederationInternalAPI)
	if !ok {
		panic("This is a programming error.")
	}
	routing.Setup(fedMux, keyMux, nil, &cfg, nil, r, keyRing, &fedClient, &userapi, nil, &base.Cfg.MSCs, nil, nil)

	handler := fedMux.Get(routing.QueryProfileRouteName).GetHandler().ServeHTTP
	_, sk, _ := ed25519.GenerateKey(nil)
	keyID := signing.KeyID
	pk := sk.Public().(ed25519.PublicKey)
	serverName := gomatrixserverlib.ServerName(hex.EncodeToString(pk))
	req := gomatrixserverlib.NewFederationRequest("GET", serverName, "remote", "/query/directory?user_id="+url.QueryEscape("@user:remote"))
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
