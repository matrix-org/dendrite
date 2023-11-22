// Copyright 2020 The Matrix.org Foundation C.I.C.
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

package httputil

import (
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/gorilla/mux"
	"github.com/matrix-org/gomatrixserverlib/spec"
)

// URLDecodeMapValues is a function that iterates through each of the items in a
// map, URL decodes the value, and returns a new map with the decoded values
// under the same key names
func URLDecodeMapValues(vmap map[string]string) (map[string]string, error) {
	decoded := make(map[string]string, len(vmap))
	for key, value := range vmap {
		decodedVal, err := url.PathUnescape(value)
		if err != nil {
			return make(map[string]string), err
		}
		decoded[key] = decodedVal
	}

	return decoded, nil
}

type Routers struct {
	Client        *mux.Router
	Federation    *mux.Router
	Keys          *mux.Router
	Media         *mux.Router
	WellKnown     *mux.Router
	Static        *mux.Router
	DendriteAdmin *mux.Router
	SynapseAdmin  *mux.Router
}

func NewRouters() Routers {
	r := Routers{
		Client:        mux.NewRouter().SkipClean(true).PathPrefix(PublicClientPathPrefix).Subrouter().UseEncodedPath(),
		Federation:    mux.NewRouter().SkipClean(true).PathPrefix(PublicFederationPathPrefix).Subrouter().UseEncodedPath(),
		Keys:          mux.NewRouter().SkipClean(true).PathPrefix(PublicKeyPathPrefix).Subrouter().UseEncodedPath(),
		Media:         mux.NewRouter().SkipClean(true).PathPrefix(PublicMediaPathPrefix).Subrouter().UseEncodedPath(),
		WellKnown:     mux.NewRouter().SkipClean(true).PathPrefix(PublicWellKnownPrefix).Subrouter().UseEncodedPath(),
		Static:        mux.NewRouter().SkipClean(true).PathPrefix(PublicStaticPath).Subrouter().UseEncodedPath(),
		DendriteAdmin: mux.NewRouter().SkipClean(true).PathPrefix(DendriteAdminPathPrefix).Subrouter().UseEncodedPath(),
		SynapseAdmin:  mux.NewRouter().SkipClean(true).PathPrefix(SynapseAdminPathPrefix).Subrouter().UseEncodedPath(),
	}
	r.configureHTTPErrors()
	return r
}

var NotAllowedHandler = WrapHandlerInCORS(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusMethodNotAllowed)
	w.Header().Set("Content-Type", "application/json")
	unrecognizedErr, _ := json.Marshal(spec.Unrecognized("Unrecognized request")) // nolint:misspell
	_, _ = w.Write(unrecognizedErr)                                               // nolint:misspell
}))

var NotFoundCORSHandler = WrapHandlerInCORS(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	w.Header().Set("Content-Type", "application/json")
	unrecognizedErr, _ := json.Marshal(spec.Unrecognized("Unrecognized request")) // nolint:misspell
	_, _ = w.Write(unrecognizedErr)                                               // nolint:misspell
}))

func (r *Routers) configureHTTPErrors() {
	for _, router := range []*mux.Router{
		r.Client, r.Federation, r.Keys,
		r.Media, r.WellKnown, r.Static,
		r.DendriteAdmin, r.SynapseAdmin,
	} {
		router.NotFoundHandler = NotFoundCORSHandler
		router.MethodNotAllowedHandler = NotAllowedHandler
	}
}
