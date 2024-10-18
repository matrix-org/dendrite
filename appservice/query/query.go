// Copyright 2018-2024 New Vector Ltd.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

// Package query handles requests from other internal dendrite components when
// they interact with the AppServiceQueryAPI.
package query

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"

	log "github.com/sirupsen/logrus"

	"github.com/element-hq/dendrite/appservice/api"
	"github.com/element-hq/dendrite/internal"
	"github.com/element-hq/dendrite/setup/config"
)

// AppServiceQueryAPI is an implementation of api.AppServiceQueryAPI
type AppServiceQueryAPI struct {
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
	trace, ctx := internal.StartRegion(ctx, "ApplicationServiceRoomAlias")
	defer trace.EndRegion()

	// Determine which application service should handle this request
	for _, appservice := range a.Cfg.Derived.ApplicationServices {
		if appservice.URL != "" && appservice.IsInterestedInRoomAlias(request.Alias) {
			path := api.ASRoomAliasExistsPath
			if a.Cfg.LegacyPaths {
				path = api.ASRoomAliasExistsLegacyPath
			}
			// The full path to the rooms API, includes hs token
			URL, err := url.Parse(appservice.RequestUrl() + path)
			if err != nil {
				return err
			}

			URL.Path += request.Alias
			if a.Cfg.LegacyAuth {
				q := URL.Query()
				q.Set("access_token", appservice.HSToken)
				URL.RawQuery = q.Encode()
			}
			apiURL := URL.String()

			// Send a request to each application service. If one responds that it has
			// created the room, immediately return.
			req, err := http.NewRequest(http.MethodGet, apiURL, nil)
			if err != nil {
				return err
			}
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", appservice.HSToken))
			req = req.WithContext(ctx)

			resp, err := appservice.HTTPClient.Do(req)
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
	trace, ctx := internal.StartRegion(ctx, "ApplicationServiceUserID")
	defer trace.EndRegion()

	// Determine which application service should handle this request
	for _, appservice := range a.Cfg.Derived.ApplicationServices {
		if appservice.URL != "" && appservice.IsInterestedInUserID(request.UserID) {
			// The full path to the rooms API, includes hs token
			path := api.ASUserExistsPath
			if a.Cfg.LegacyPaths {
				path = api.ASUserExistsLegacyPath
			}
			URL, err := url.Parse(appservice.RequestUrl() + path)
			if err != nil {
				return err
			}
			URL.Path += request.UserID
			if a.Cfg.LegacyAuth {
				q := URL.Query()
				q.Set("access_token", appservice.HSToken)
				URL.RawQuery = q.Encode()
			}
			apiURL := URL.String()

			// Send a request to each application service. If one responds that it has
			// created the user, immediately return.
			req, err := http.NewRequest(http.MethodGet, apiURL, nil)
			if err != nil {
				return err
			}
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", appservice.HSToken))
			resp, err := appservice.HTTPClient.Do(req.WithContext(ctx))
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

func requestDo[T thirdpartyResponses](as *config.ApplicationService, url string, response *T) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", as.HSToken))
	resp, err := as.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close() // nolint: errcheck
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, &response)
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

	path := api.ASLocationPath
	if a.Cfg.LegacyPaths {
		path = api.ASLocationLegacyPath
	}
	for _, as := range a.Cfg.Derived.ApplicationServices {
		var asLocations []api.ASLocationResponse
		if a.Cfg.LegacyAuth {
			params.Set("access_token", as.HSToken)
		}

		url := as.RequestUrl() + path
		if req.Protocol != "" {
			url += "/" + req.Protocol
		}

		if err := requestDo[[]api.ASLocationResponse](&as, url+"?"+params.Encode(), &asLocations); err != nil {
			log.WithError(err).WithField("application_service", as.ID).Error("unable to get 'locations' from application service")
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

	path := api.ASUserPath
	if a.Cfg.LegacyPaths {
		path = api.ASUserLegacyPath
	}
	for _, as := range a.Cfg.Derived.ApplicationServices {
		var asUsers []api.ASUserResponse
		if a.Cfg.LegacyAuth {
			params.Set("access_token", as.HSToken)
		}

		url := as.RequestUrl() + path
		if req.Protocol != "" {
			url += "/" + req.Protocol
		}

		if err := requestDo[[]api.ASUserResponse](&as, url+"?"+params.Encode(), &asUsers); err != nil {
			log.WithError(err).WithField("application_service", as.ID).Error("unable to get 'user' from application service")
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
	protocolPath := api.ASProtocolPath
	if a.Cfg.LegacyPaths {
		protocolPath = api.ASProtocolLegacyPath
	}

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
			if err := requestDo[api.ASProtocolResponse](&as, as.RequestUrl()+protocolPath+req.Protocol, &proto); err != nil {
				log.WithError(err).WithField("application_service", as.ID).Error("unable to get 'protocol' from application service")
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
			if err := requestDo[api.ASProtocolResponse](&as, as.RequestUrl()+protocolPath+p, &proto); err != nil {
				log.WithError(err).WithField("application_service", as.ID).Error("unable to get 'protocol' from application service")
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
