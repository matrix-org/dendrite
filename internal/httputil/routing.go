// Copyright 2024 New Vector Ltd.
// Copyright 2020 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

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
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusMethodNotAllowed)
	unrecognizedErr, _ := json.Marshal(spec.Unrecognized("Unrecognized request")) // nolint:misspell
	_, _ = w.Write(unrecognizedErr)                                               // nolint:misspell
}))

var NotFoundCORSHandler = WrapHandlerInCORS(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
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
