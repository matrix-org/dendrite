// Copyright 2024 New Vector Ltd.
// Copyright 2018 Vector Creations Ltd
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package appservice

import (
	"context"
	"sync"

	"github.com/element-hq/dendrite/setup/jetstream"
	"github.com/element-hq/dendrite/setup/process"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/sirupsen/logrus"

	appserviceAPI "github.com/element-hq/dendrite/appservice/api"
	"github.com/element-hq/dendrite/appservice/consumers"
	"github.com/element-hq/dendrite/appservice/query"
	roomserverAPI "github.com/element-hq/dendrite/roomserver/api"
	"github.com/element-hq/dendrite/setup/config"
	userapi "github.com/element-hq/dendrite/userapi/api"
)

// NewInternalAPI returns a concerete implementation of the internal API. Callers
// can call functions directly on the returned API or via an HTTP interface using AddInternalRoutes.
func NewInternalAPI(
	processContext *process.ProcessContext,
	cfg *config.Dendrite,
	natsInstance *jetstream.NATSInstance,
	userAPI userapi.AppserviceUserAPI,
	rsAPI roomserverAPI.RoomserverInternalAPI,
) appserviceAPI.AppServiceInternalAPI {

	// Create appserivce query API with an HTTP client that will be used for all
	// outbound and inbound requests (inbound only for the internal API)
	appserviceQueryAPI := &query.AppServiceQueryAPI{
		Cfg:           &cfg.AppServiceAPI,
		ProtocolCache: map[string]appserviceAPI.ASProtocolResponse{},
		CacheMu:       sync.Mutex{},
	}

	if len(cfg.Derived.ApplicationServices) == 0 {
		return appserviceQueryAPI
	}

	// Wrap application services in a type that relates the application service and
	// a sync.Cond object that can be used to notify workers when there are new
	// events to be sent out.
	for _, appservice := range cfg.Derived.ApplicationServices {
		// Create bot account for this AS if it doesn't already exist
		if err := generateAppServiceAccount(userAPI, appservice, cfg.Global.ServerName); err != nil {
			logrus.WithFields(logrus.Fields{
				"appservice": appservice.ID,
			}).WithError(err).Panicf("failed to generate bot account for appservice")
		}
	}

	// Only consume if we actually have ASes to track, else we'll just chew cycles needlessly.
	// We can't add ASes at runtime so this is safe to do.
	js, _ := natsInstance.Prepare(processContext, &cfg.Global.JetStream)
	consumer := consumers.NewOutputRoomEventConsumer(
		processContext, &cfg.AppServiceAPI,
		js, rsAPI,
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
	serverName spec.ServerName,
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
