// Copyright 2023 The Matrix.org Foundation C.I.C.
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

package relay

import (
	"context"
	"testing"
	"time"

	federationAPI "github.com/matrix-org/dendrite/federationapi/api"
	relayServerAPI "github.com/matrix-org/dendrite/relayapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"gotest.tools/v3/poll"
)

var testRelayServers = []gomatrixserverlib.ServerName{"relay1", "relay2"}

type FakeFedAPI struct {
	federationAPI.FederationInternalAPI
}

func (f *FakeFedAPI) P2PQueryRelayServers(
	ctx context.Context,
	req *federationAPI.P2PQueryRelayServersRequest,
	res *federationAPI.P2PQueryRelayServersResponse,
) error {
	res.RelayServers = testRelayServers
	return nil
}

type FakeRelayAPI struct {
	relayServerAPI.RelayInternalAPI
}

func (r *FakeRelayAPI) PerformRelayServerSync(
	ctx context.Context,
	userID gomatrixserverlib.UserID,
	relayServer gomatrixserverlib.ServerName,
) error {
	return nil
}

func TestRelayRetrieverInitialization(t *testing.T) {
	retriever := NewRelayServerRetriever(
		context.Background(),
		"server",
		&FakeFedAPI{},
		&FakeRelayAPI{},
		make(chan bool),
	)

	retriever.InitializeRelayServers(logrus.WithField("test", "relay"))
	relayServers := retriever.GetQueriedServerStatus()
	assert.Equal(t, 2, len(relayServers))
}

func TestRelayRetrieverSync(t *testing.T) {
	retriever := NewRelayServerRetriever(
		context.Background(),
		"server",
		&FakeFedAPI{},
		&FakeRelayAPI{},
		make(chan bool),
	)

	retriever.InitializeRelayServers(logrus.WithField("test", "relay"))
	relayServers := retriever.GetQueriedServerStatus()
	assert.Equal(t, 2, len(relayServers))

	stopRelayServerSync := make(chan bool)
	go retriever.SyncRelayServers(stopRelayServerSync)

	check := func(log poll.LogT) poll.Result {
		relayServers := retriever.GetQueriedServerStatus()
		for _, queried := range relayServers {
			if !queried {
				return poll.Continue("waiting for all servers to be queried")
			}
		}

		stopRelayServerSync <- true
		return poll.Success()
	}
	poll.WaitOn(t, check, poll.WithTimeout(5*time.Second), poll.WithDelay(100*time.Millisecond))
}
