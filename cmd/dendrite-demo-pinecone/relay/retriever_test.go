// Copyright 2024 New Vector Ltd.
// Copyright 2023 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package relay

import (
	"context"
	"testing"
	"time"

	federationAPI "github.com/element-hq/dendrite/federationapi/api"
	relayServerAPI "github.com/element-hq/dendrite/relayapi/api"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"gotest.tools/v3/poll"
)

var testRelayServers = []spec.ServerName{"relay1", "relay2"}

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
	userID spec.UserID,
	relayServer spec.ServerName,
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
