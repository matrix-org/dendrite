// Copyright 2018 Vector Creations Ltd
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

package appservice

import (
	"context"
	"crypto/tls"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"

	appserviceAPI "github.com/matrix-org/dendrite/appservice/api"
	"github.com/matrix-org/dendrite/appservice/consumers"
	"github.com/matrix-org/dendrite/appservice/inthttp"
	"github.com/matrix-org/dendrite/appservice/query"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/base"
	"github.com/matrix-org/dendrite/setup/config"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
)

// AddInternalRoutes registers HTTP handlers for internal API calls
func AddInternalRoutes(router *mux.Router, queryAPI appserviceAPI.AppServiceInternalAPI) {
	inthttp.AddRoutes(queryAPI, router)
}

// NewInternalAPI returns a concerete implementation of the internal API. Callers
// can call functions directly on the returned API or via an HTTP interface using AddInternalRoutes.
func NewInternalAPI(
	base *base.BaseDendrite,
	userAPI userapi.UserInternalAPI,
	rsAPI roomserverAPI.RoomserverInternalAPI,
) appserviceAPI.AppServiceInternalAPI {
	client := &http.Client{
		Timeout: time.Second * 30,
		Transport: &http.Transport{
			DisableKeepAlives: true,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: base.Cfg.AppServiceAPI.DisableTLSValidation,
			},
			Proxy: http.ProxyFromEnvironment,
		},
	}
	// Create appserivce query API with an HTTP client that will be used for all
	// outbound and inbound requests (inbound only for the internal API)
	appserviceQueryAPI := &query.AppServiceQueryAPI{
		HTTPClient:    client,
		Cfg:           &base.Cfg.AppServiceAPI,
		ProtocolCache: map[string]appserviceAPI.ASProtocolResponse{},
		CacheMu:       sync.Mutex{},
	}

	if len(base.Cfg.Derived.ApplicationServices) == 0 {
		return appserviceQueryAPI
	}

	// Wrap application services in a type that relates the application service and
	// a sync.Cond object that can be used to notify workers when there are new
	// events to be sent out.
	for _, appservice := range base.Cfg.Derived.ApplicationServices {
		// Create bot account for this AS if it doesn't already exist
		if err := generateAppServiceAccount(userAPI, appservice, base.Cfg.Global.ServerName); err != nil {
			logrus.WithFields(logrus.Fields{
				"appservice": appservice.ID,
			}).WithError(err).Panicf("failed to generate bot account for appservice")
		}
	}

	// Only consume if we actually have ASes to track, else we'll just chew cycles needlessly.
	// We can't add ASes at runtime so this is safe to do.
	js, _ := base.NATS.Prepare(base.ProcessContext, &base.Cfg.Global.JetStream)
	consumer := consumers.NewOutputRoomEventConsumer(
		base.ProcessContext, &base.Cfg.AppServiceAPI,
		client, js, rsAPI,
	)
	if err := consumer.Start(); err != nil {
		logrus.WithError(err).Panicf("failed to start appservice roomserver consumer")
	}

	return appserviceQueryAPI
}

// generateAppServiceAccounts creates a dummy account based off the
// `sender_localpart` field of each application service if it doesn't
// exist already
func generateAppServiceAccount(
	userAPI userapi.AppserviceUserAPI,
	as config.ApplicationService,
	serverName gomatrixserverlib.ServerName,
) error {
	var accRes userapi.PerformAccountCreationResponse
	err := userAPI.PerformAccountCreation(context.Background(), &userapi.PerformAccountCreationRequest{
		AccountType:  userapi.AccountTypeAppService,
		Localpart:    as.SenderLocalpart,
		ServerName:   serverName,
		AppServiceID: as.ID,
		OnConflict:   userapi.ConflictUpdate,
	}, &accRes)
	if err != nil {
		return err
	}
	var devRes userapi.PerformDeviceCreationResponse
	err = userAPI.PerformDeviceCreation(context.Background(), &userapi.PerformDeviceCreationRequest{
		Localpart:          as.SenderLocalpart,
		ServerName:         serverName,
		AccessToken:        as.ASToken,
		DeviceID:           &as.SenderLocalpart,
		DeviceDisplayName:  &as.SenderLocalpart,
		NoDeviceListUpdate: true,
	}, &devRes)
	return err
}
