// Copyright 2022 The Matrix.org Foundation C.I.C.
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

package relayapi

import (
	"context"

	"github.com/matrix-org/dendrite/federationapi/producers"
	"github.com/matrix-org/dendrite/internal"
	rsAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"
)

type RelayAPI struct {
	fedClient              *gomatrixserverlib.FederationClient
	rsAPI                  rsAPI.RoomserverInternalAPI
	keyRing                *gomatrixserverlib.KeyRing
	producer               *producers.SyncAPIProducer
	presenceEnabledInbound bool
	serverName             gomatrixserverlib.ServerName
}

func NewRelayAPI(
	fedClient *gomatrixserverlib.FederationClient,
	rsAPI rsAPI.RoomserverInternalAPI,
	keyRing *gomatrixserverlib.KeyRing,
	producer *producers.SyncAPIProducer,
	presenceEnabledInbound bool,
	serverName gomatrixserverlib.ServerName,
) RelayAPI {
	return RelayAPI{
		fedClient:              fedClient,
		rsAPI:                  rsAPI,
		keyRing:                keyRing,
		producer:               producer,
		presenceEnabledInbound: presenceEnabledInbound,
		serverName:             serverName,
	}
}

// PerformRelayServerSync implements api.FederationInternalAPI
func (r *RelayAPI) PerformRelayServerSync(userID gomatrixserverlib.UserID, relayServer gomatrixserverlib.ServerName) error {
	asyncResponse, err := r.fedClient.GetAsyncEvents(context.Background(), userID, relayServer)
	if err != nil {
		logrus.Errorf("GetAsyncEvents: %s", err.Error())
		return err
	}
	r.processTransaction(&asyncResponse.Transaction)

	for asyncResponse.Remaining > 0 {
		asyncResponse, err := r.fedClient.GetAsyncEvents(context.Background(), userID, relayServer)
		if err != nil {
			logrus.Errorf("GetAsyncEvents: %s", err.Error())
			return err
		}
		r.processTransaction(&asyncResponse.Transaction)
	}

	return nil
}

func (r *RelayAPI) processTransaction(txn *gomatrixserverlib.Transaction) {
	logrus.Warn("Processing transaction from relay server")
	mu := internal.NewMutexByRoom()
	t := internal.NewTxnReq(
		r.rsAPI,
		nil,
		r.serverName,
		r.keyRing,
		mu,
		r.producer,
		r.presenceEnabledInbound,
		txn.PDUs,
		txn.EDUs,
		txn.Origin,
		txn.TransactionID,
		txn.Destination)

	t.ProcessTransaction(context.TODO())
}
