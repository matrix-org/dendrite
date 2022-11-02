// Copyright 2018 New Vector Ltd
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

// Package query handles requests from other internal dendrite components when
// they interact with the AppServiceQueryAPI.
package query

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/opentracing/opentracing-go"
	log "github.com/sirupsen/logrus"

	"github.com/matrix-org/dendrite/appservice/api"
	"github.com/matrix-org/dendrite/setup/config"
)

const roomAliasExistsPath = "/rooms/"
const userIDExistsPath = "/users/"

// AppServiceQueryAPI is an implementation of api.AppServiceQueryAPI
type AppServiceQueryAPI struct {
	HTTPClient    *http.Client
	Cfg           *config.AppServiceAPI
	ProtocolCache map[string]api.ASProtocolResponse
	CacheMu       sync.Mutex
}

// RoomAliasExists performs a request to '/room/{roomAlias}' on all known
// handling application services until one admits to owning the room
func (a *AppServiceQueryAPI) RoomAliasExists(
	ctx context.Context,
	request *api.RoomAliasExistsRequest,
	response *api.RoomAliasExistsResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "ApplicationServiceRoomAlias")
	defer span.Finish()

	// Determine which application service should handle this request
	for _, appservice := range a.Cfg.Derived.ApplicationServices {
		if appservice.URL != "" && appservice.IsInterestedInRoomAlias(request.Alias) {
			// The full path to the rooms API, includes hs token
			URL, err := url.Parse(appservice.URL + roomAliasExistsPath)
			if err != nil {
				return err
			}

			URL.Path += request.Alias
			apiURL := URL.String() + "?access_token=" + appservice.HSToken

			// Send a request to each application service. If one responds that it has
			// created the room, immediately return.
			req, err := http.NewRequest(http.MethodGet, apiURL, nil)
			if err != nil {
				return err
			}
			req = req.WithContext(ctx)

			resp, err := a.HTTPClient.Do(req)
			if resp != nil {
				defer func() {
					err = resp.Body.Close()
					if err != nil {
						log.WithFields(log.Fields{
							"appservice_id": appservice.ID,
							"status_code":   resp.StatusCode,
						}).WithError(err).Error("Unable to close application service response body")
					}
				}()
			}
			if err != nil {
				log.WithError(err).Errorf("Issue querying room alias on application service %s", appservice.ID)
				return err
			}
			switch resp.StatusCode {
			case http.StatusOK:
				// OK received from appservice. Room exists
				response.AliasExists = true
				return nil
			case http.StatusNotFound:
				// Room does not exist
			default:
				// Application service reported an error. Warn
				log.WithFields(log.Fields{
					"appservice_id": appservice.ID,
					"status_code":   resp.StatusCode,
				}).Warn("Application service responded with non-OK status code")
			}
		}
	}

	response.AliasExists = false
	return nil
}

// UserIDExists performs a request to '/users/{userID}' on all known
// handling application services until one admits to owning the user ID
func (a *AppServiceQueryAPI) UserIDExists(
	ctx context.Context,
	request *api.UserIDExistsRequest,
	response *api.UserIDExistsResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "ApplicationServiceUserID")
	defer span.Finish()

	// Determine which application service should handle this request
	for _, appservice := range a.Cfg.Derived.ApplicationServices {
		if appservice.URL != "" && appservice.IsInterestedInUserID(request.UserID) {
			// The full path to the rooms API, includes hs token
			URL, err := url.Parse(appservice.URL + userIDExistsPath)
			if err != nil {
				return err
			}
			URL.Path += request.UserID
			apiURL := URL.String() + "?access_token=" + appservice.HSToken

			// Send a request to each application service. If one responds that it has
			// created the user, immediately return.
			req, err := http.NewRequest(http.MethodGet, apiURL, nil)
			if err != nil {
				return err
			}
			resp, err := a.HTTPClient.Do(req.WithContext(ctx))
			if resp != nil {
				defer func() {
					err = resp.Body.Close()
					if err != nil {
						log.WithFields(log.Fields{
							"appservice_id": appservice.ID,
							"status_code":   resp.StatusCode,
						}).Error("Unable to close application service response body")
					}
				}()
			}
			if err != nil {
				log.WithFields(log.Fields{
					"appservice_id": appservice.ID,
				}).WithError(err).Error("issue querying user ID on application service")
				return err
			}
			if resp.StatusCode == http.StatusOK {
				// StatusOK received from appservice. User ID exists
				response.UserIDExists = true
				return nil
			}

			// Log non OK
			log.WithFields(log.Fields{
				"appservice_id": appservice.ID,
				"status_code":   resp.StatusCode,
			}).Warn("application service responded with non-OK status code")
		}
	}

	response.UserIDExists = false
	return nil
}

type thirdpartyResponses interface {
	api.ASProtocolResponse | []api.ASUserResponse | []api.ASLocationResponse
}

func requestDo[T thirdpartyResponses](client *http.Client, url string, response *T) (err error) {
	origURL := url
	// try v1 and unstable appservice endpoints
	for _, version := range []string{"v1", "unstable"} {
		var resp *http.Response
		var body []byte
		asURL := strings.Replace(origURL, "unstable", version, 1)
		resp, err = client.Get(asURL)
		if err != nil {
			continue
		}
		defer resp.Body.Close() // nolint: errcheck
		body, err = io.ReadAll(resp.Body)
		if err != nil {
			continue
		}
		return json.Unmarshal(body, &response)
	}
	return err
}

func (a *AppServiceQueryAPI) Locations(
	ctx context.Context,
	req *api.LocationRequest,
	resp *api.LocationResponse,
) error {
	params, err := url.ParseQuery(req.Params)
	if err != nil {
		return err
	}

	for _, as := range a.Cfg.Derived.ApplicationServices {
		var asLocations []api.ASLocationResponse
		params.Set("access_token", as.HSToken)

		url := as.URL + api.ASLocationPath
		if req.Protocol != "" {
			url += "/" + req.Protocol
		}

		if err := requestDo[[]api.ASLocationResponse](a.HTTPClient, url+"?"+params.Encode(), &asLocations); err != nil {
			log.WithError(err).Error("unable to get 'locations' from application service")
			continue
		}

		resp.Locations = append(resp.Locations, asLocations...)
	}

	if len(resp.Locations) == 0 {
		resp.Exists = false
		return nil
	}
	resp.Exists = true
	return nil
}

func (a *AppServiceQueryAPI) User(
	ctx context.Context,
	req *api.UserRequest,
	resp *api.UserResponse,
) error {
	params, err := url.ParseQuery(req.Params)
	if err != nil {
		return err
	}

	for _, as := range a.Cfg.Derived.ApplicationServices {
		var asUsers []api.ASUserResponse
		params.Set("access_token", as.HSToken)

		url := as.URL + api.ASUserPath
		if req.Protocol != "" {
			url += "/" + req.Protocol
		}

		if err := requestDo[[]api.ASUserResponse](a.HTTPClient, url+"?"+params.Encode(), &asUsers); err != nil {
			log.WithError(err).Error("unable to get 'user' from application service")
			continue
		}

		resp.Users = append(resp.Users, asUsers...)
	}

	if len(resp.Users) == 0 {
		resp.Exists = false
		return nil
	}
	resp.Exists = true
	return nil
}

func (a *AppServiceQueryAPI) Protocols(
	ctx context.Context,
	req *api.ProtocolRequest,
	resp *api.ProtocolResponse,
) error {

	// get a single protocol response
	if req.Protocol != "" {

		a.CacheMu.Lock()
		defer a.CacheMu.Unlock()
		if proto, ok := a.ProtocolCache[req.Protocol]; ok {
			resp.Exists = true
			resp.Protocols = map[string]api.ASProtocolResponse{
				req.Protocol: proto,
			}
			return nil
		}

		response := api.ASProtocolResponse{}
		for _, as := range a.Cfg.Derived.ApplicationServices {
			var proto api.ASProtocolResponse
			if err := requestDo[api.ASProtocolResponse](a.HTTPClient, as.URL+api.ASProtocolPath+req.Protocol, &proto); err != nil {
				log.WithError(err).Error("unable to get 'protocol' from application service")
				continue
			}

			if len(response.Instances) != 0 {
				response.Instances = append(response.Instances, proto.Instances...)
			} else {
				response = proto
			}
		}

		if len(response.Instances) == 0 {
			resp.Exists = false
			return nil
		}

		resp.Exists = true
		resp.Protocols = map[string]api.ASProtocolResponse{
			req.Protocol: response,
		}
		a.ProtocolCache[req.Protocol] = response
		return nil
	}

	response := make(map[string]api.ASProtocolResponse, len(a.Cfg.Derived.ApplicationServices))

	for _, as := range a.Cfg.Derived.ApplicationServices {
		for _, p := range as.Protocols {
			var proto api.ASProtocolResponse
			if err := requestDo[api.ASProtocolResponse](a.HTTPClient, as.URL+api.ASProtocolPath+p, &proto); err != nil {
				log.WithError(err).Error("unable to get 'protocol' from application service")
				continue
			}
			existing, ok := response[p]
			if !ok {
				response[p] = proto
				continue
			}
			existing.Instances = append(existing.Instances, proto.Instances...)
			response[p] = existing
		}
	}

	if len(response) == 0 {
		resp.Exists = false
		return nil
	}

	a.CacheMu.Lock()
	defer a.CacheMu.Unlock()
	a.ProtocolCache = response

	resp.Exists = true
	resp.Protocols = response
	return nil
}
