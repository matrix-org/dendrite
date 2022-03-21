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
	"github.com/matrix-org/dendrite/appservice/storage"
	"github.com/matrix-org/dendrite/appservice/types"
	"github.com/matrix-org/dendrite/appservice/workers"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/base"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/jetstream"
	userapi "github.com/matrix-org/dendrite/userapi/api"
)

// AddInternalRoutes registers HTTP handlers for internal API calls
func AddInternalRoutes(router *mux.Router, queryAPI appserviceAPI.AppServiceQueryAPI) {
	inthttp.AddRoutes(queryAPI, router)
}

// NewInternalAPI returns a concerete implementation of the internal API. Callers
// can call functions directly on the returned API or via an HTTP interface using AddInternalRoutes.
func NewInternalAPI(
	base *base.BaseDendrite,
	userAPI userapi.UserInternalAPI,
	rsAPI roomserverAPI.RoomserverInternalAPI,
) appserviceAPI.AppServiceQueryAPI {
	client := &http.Client{
		Timeout: time.Second * 30,
		Transport: &http.Transport{
			DisableKeepAlives: true,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: base.Cfg.AppServiceAPI.DisableTLSValidation,
			},
		},
	}
	js, _ := jetstream.Prepare(base.ProcessContext, &base.Cfg.Global.JetStream)

	// Create a connection to the appservice postgres DB
	appserviceDB, err := storage.NewDatabase(&base.Cfg.AppServiceAPI.Database)
	if err != nil {
		logrus.WithError(err).Panicf("failed to connect to appservice db")
	}

	// Wrap application services in a type that relates the application service and
	// a sync.Cond object that can be used to notify workers when there are new
	// events to be sent out.
	workerStates := make([]types.ApplicationServiceWorkerState, len(base.Cfg.Derived.ApplicationServices))
	for i, appservice := range base.Cfg.Derived.ApplicationServices {
		m := sync.Mutex{}
		ws := types.ApplicationServiceWorkerState{
			AppService: appservice,
			Cond:       sync.NewCond(&m),
		}
		workerStates[i] = ws

		// Create bot account for this AS if it doesn't already exist
		if err = generateAppServiceAccount(userAPI, appservice); err != nil {
			logrus.WithFields(logrus.Fields{
				"appservice": appservice.ID,
			}).WithError(err).Panicf("failed to generate bot account for appservice")
		}
	}

	// Create appserivce query API with an HTTP client that will be used for all
	// outbound and inbound requests (inbound only for the internal API)
	appserviceQueryAPI := &query.AppServiceQueryAPI{
		HTTPClient: client,
		Cfg:        base.Cfg,
	}

	// Only consume if we actually have ASes to track, else we'll just chew cycles needlessly.
	// We can't add ASes at runtime so this is safe to do.
	if len(workerStates) > 0 {
		consumer := consumers.NewOutputRoomEventConsumer(
			base.ProcessContext, base.Cfg, js, appserviceDB,
			rsAPI, workerStates,
		)
		if err := consumer.Start(); err != nil {
			logrus.WithError(err).Panicf("failed to start appservice roomserver consumer")
		}
	}

	// Create application service transaction workers
	if err := workers.SetupTransactionWorkers(client, appserviceDB, workerStates); err != nil {
		logrus.WithError(err).Panicf("failed to start app service transaction workers")
	}
	return appserviceQueryAPI
}

// generateAppServiceAccounts creates a dummy account based off the
// `sender_localpart` field of each application service if it doesn't
// exist already
func generateAppServiceAccount(
	userAPI userapi.UserInternalAPI,
	as config.ApplicationService,
) error {
	var accRes userapi.PerformAccountCreationResponse
	err := userAPI.PerformAccountCreation(context.Background(), &userapi.PerformAccountCreationRequest{
		AccountType:  userapi.AccountTypeAppService,
		Localpart:    as.SenderLocalpart,
		AppServiceID: as.ID,
		OnConflict:   userapi.ConflictUpdate,
	}, &accRes)
	if err != nil {
		return err
	}
	var devRes userapi.PerformDeviceCreationResponse
	err = userAPI.PerformDeviceCreation(context.Background(), &userapi.PerformDeviceCreationRequest{
		Localpart:          as.SenderLocalpart,
		AccessToken:        as.ASToken,
		DeviceID:           &as.SenderLocalpart,
		DeviceDisplayName:  &as.SenderLocalpart,
		NoDeviceListUpdate: true,
	}, &devRes)
	return err
}
