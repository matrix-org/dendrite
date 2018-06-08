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
	"net/http"
	"sync"
	"time"

	appserviceAPI "github.com/matrix-org/dendrite/appservice/api"
	"github.com/matrix-org/dendrite/appservice/consumers"
	"github.com/matrix-org/dendrite/appservice/query"
	"github.com/matrix-org/dendrite/appservice/routing"
	"github.com/matrix-org/dendrite/appservice/storage"
	"github.com/matrix-org/dendrite/appservice/types"
	"github.com/matrix-org/dendrite/appservice/workers"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"
	"github.com/matrix-org/dendrite/common/basecomponent"
	"github.com/matrix-org/dendrite/common/transactions"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"
)

// SetupAppServiceAPIComponent sets up and registers HTTP handlers for the AppServices
// component.
func SetupAppServiceAPIComponent(
	base *basecomponent.BaseDendrite,
	accountsDB *accounts.Database,
	federation *gomatrixserverlib.FederationClient,
	roomserverAliasAPI roomserverAPI.RoomserverAliasAPI,
	roomserverQueryAPI roomserverAPI.RoomserverQueryAPI,
	transactionsCache *transactions.Cache,
) appserviceAPI.AppServiceQueryAPI {
	// Create a connection to the appservice postgres DB
	appserviceDB, err := storage.NewDatabase(string(base.Cfg.Database.AppService))
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
	}

	// Create a HTTP client that this component will use for all outbound and
	// inbound requests (inbound only for the internal API)
	httpClient := &http.Client{
		Timeout: time.Second * 30,
	}

	appserviceQueryAPI := query.AppServiceQueryAPI{
		HTTPClient: httpClient,
		Cfg:        base.Cfg,
	}

	appserviceQueryAPI.SetupHTTP(http.DefaultServeMux)

	consumer := consumers.NewOutputRoomEventConsumer(
		base.Cfg, base.KafkaConsumer, accountsDB, appserviceDB,
		roomserverQueryAPI, roomserverAliasAPI, workerStates,
	)
	if err := consumer.Start(); err != nil {
		logrus.WithError(err).Panicf("failed to start app service roomserver consumer")
	}

	// Create application service transaction workers
	if err := workers.SetupTransactionWorkers(appserviceDB, workerStates); err != nil {
		logrus.WithError(err).Panicf("failed to start app service transaction workers")
	}

	// Set up HTTP Endpoints
	routing.Setup(
		base.APIMux, *base.Cfg, roomserverQueryAPI, roomserverAliasAPI,
		accountsDB, federation, transactionsCache,
	)

	return &appserviceQueryAPI
}
