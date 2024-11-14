// Copyright 2024 New Vector Ltd.
// Copyright 2022 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package users

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/element-hq/dendrite/clientapi/auth/authtypes"
	clienthttputil "github.com/element-hq/dendrite/clientapi/httputil"
	"github.com/element-hq/dendrite/cmd/dendrite-demo-pinecone/defaults"
	userapi "github.com/element-hq/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"

	pineconeRouter "github.com/matrix-org/pinecone/router"
	pineconeSessions "github.com/matrix-org/pinecone/sessions"
)

type PineconeUserProvider struct {
	r         *pineconeRouter.Router
	s         *pineconeSessions.Sessions
	userAPI   userapi.QuerySearchProfilesAPI
	fedClient fclient.FederationClient
}

const PublicURL = "/_matrix/p2p/profiles"

func NewPineconeUserProvider(
	r *pineconeRouter.Router,
	s *pineconeSessions.Sessions,
	userAPI userapi.QuerySearchProfilesAPI,
	fedClient fclient.FederationClient,
) *PineconeUserProvider {
	p := &PineconeUserProvider{
		r:         r,
		s:         s,
		userAPI:   userAPI,
		fedClient: fedClient,
	}
	return p
}

func (p *PineconeUserProvider) FederatedUserProfiles(w http.ResponseWriter, r *http.Request) {
	req := &userapi.QuerySearchProfilesRequest{Limit: 25}
	res := &userapi.QuerySearchProfilesResponse{}
	if err := clienthttputil.UnmarshalJSONRequest(r, &req); err != nil {
		w.WriteHeader(400)
		return
	}
	if err := p.userAPI.QuerySearchProfiles(r.Context(), req, res); err != nil {
		w.WriteHeader(400)
		return
	}
	j, err := json.Marshal(res)
	if err != nil {
		w.WriteHeader(400)
		return
	}
	w.WriteHeader(200)
	_, _ = w.Write(j)
}

func (p *PineconeUserProvider) QuerySearchProfiles(ctx context.Context, req *userapi.QuerySearchProfilesRequest, res *userapi.QuerySearchProfilesResponse) error {
	list := map[spec.ServerName]struct{}{}
	for k := range defaults.DefaultServerNames {
		list[k] = struct{}{}
	}
	for _, k := range p.r.Peers() {
		list[spec.ServerName(k.PublicKey)] = struct{}{}
	}
	res.Profiles = bulkFetchUserDirectoriesFromServers(context.Background(), req, p.fedClient, list)
	return nil
}

// bulkFetchUserDirectoriesFromServers fetches users from the list of homeservers.
// Returns a list of user profiles.
func bulkFetchUserDirectoriesFromServers(
	ctx context.Context, req *userapi.QuerySearchProfilesRequest,
	fedClient fclient.FederationClient,
	homeservers map[spec.ServerName]struct{},
) (profiles []authtypes.Profile) {
	jsonBody, err := json.Marshal(req)
	if err != nil {
		return nil
	}

	limit := 200
	// follow pipeline semantics, see https://blog.golang.org/pipelines for more info.
	// goroutines send rooms to this channel
	profileCh := make(chan authtypes.Profile, int(limit))
	// signalling channel to tell goroutines to stop sending rooms and quit
	done := make(chan bool)
	// signalling to say when we can close the room channel
	var wg sync.WaitGroup
	wg.Add(len(homeservers))
	// concurrently query for public rooms
	reqctx, reqcancel := context.WithTimeout(ctx, time.Second*5)
	for hs := range homeservers {
		go func(homeserverDomain spec.ServerName) {
			defer wg.Done()
			util.GetLogger(reqctx).WithField("hs", homeserverDomain).Info("Querying HS for users")

			jsonBodyReader := bytes.NewBuffer(jsonBody)
			httpReq, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("matrix://%s%s", homeserverDomain, PublicURL), jsonBodyReader)
			if err != nil {
				util.GetLogger(reqctx).WithError(err).WithField("hs", homeserverDomain).Warn(
					"bulkFetchUserDirectoriesFromServers: failed to create request",
				)
			}
			res := &userapi.QuerySearchProfilesResponse{}
			if err = fedClient.DoRequestAndParseResponse(reqctx, httpReq, res); err != nil {
				util.GetLogger(reqctx).WithError(err).WithField("hs", homeserverDomain).Warn(
					"bulkFetchUserDirectoriesFromServers: failed to query hs",
				)
				return
			}
			for _, profile := range res.Profiles {
				profile.ServerName = string(homeserverDomain)
				// atomically send a room or stop
				select {
				case profileCh <- profile:
				case <-done:
				case <-reqctx.Done():
					util.GetLogger(reqctx).WithError(err).WithField("hs", homeserverDomain).Info("Interrupted whilst sending profiles")
					return
				}
			}
		}(hs)
	}

	select {
	case <-time.After(5 * time.Second):
	default:
		wg.Wait()
	}
	reqcancel()
	close(done)
	close(profileCh)

	for profile := range profileCh {
		profiles = append(profiles, profile)
	}

	return profiles
}
