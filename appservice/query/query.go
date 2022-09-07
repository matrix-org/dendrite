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
	"net/http"
	"net/url"

	"github.com/matrix-org/dendrite/appservice/api"
	"github.com/matrix-org/dendrite/setup/config"
	opentracing "github.com/opentracing/opentracing-go"
	log "github.com/sirupsen/logrus"
)

const roomAliasExistsPath = "/rooms/"
const userIDExistsPath = "/users/"

// AppServiceQueryAPI is an implementation of api.AppServiceQueryAPI
type AppServiceQueryAPI struct {
	HTTPClient *http.Client
	Cfg        *config.AppServiceAPI
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
