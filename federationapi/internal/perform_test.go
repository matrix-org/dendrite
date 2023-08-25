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

package internal

import (
	"context"
	"crypto/ed25519"
	"testing"

	"github.com/matrix-org/dendrite/federationapi/api"
	"github.com/matrix-org/dendrite/federationapi/queue"
	"github.com/matrix-org/dendrite/federationapi/statistics"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/process"
	"github.com/matrix-org/dendrite/test"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/stretchr/testify/assert"
)

type testFedClient struct {
	fclient.FederationClient
	queryKeysCalled bool
	claimKeysCalled bool
	shouldFail      bool
}

func (t *testFedClient) LookupRoomAlias(ctx context.Context, origin, s spec.ServerName, roomAlias string) (res fclient.RespDirectory, err error) {
	return fclient.RespDirectory{}, nil
}

func TestPerformWakeupServers(t *testing.T) {
	testDB := test.NewInMemoryFederationDatabase()

	server := spec.ServerName("wakeup")
	testDB.AddServerToBlacklist(server)
	testDB.SetServerAssumedOffline(context.Background(), server)
	blacklisted, err := testDB.IsServerBlacklisted(server)
	assert.NoError(t, err)
	assert.True(t, blacklisted)
	offline, err := testDB.IsServerAssumedOffline(context.Background(), server)
	assert.NoError(t, err)
	assert.True(t, offline)

	_, key, err := ed25519.GenerateKey(nil)
	assert.NoError(t, err)
	cfg := config.FederationAPI{
		Matrix: &config.Global{
			SigningIdentity: fclient.SigningIdentity{
				ServerName: "relay",
				KeyID:      "ed25519:1",
				PrivateKey: key,
			},
		},
	}
	fedClient := &testFedClient{}
	stats := statistics.NewStatistics(testDB, FailuresUntilBlacklist, FailuresUntilAssumedOffline)
	queues := queue.NewOutgoingQueues(
		testDB, process.NewProcessContext(),
		false,
		cfg.Matrix.ServerName, fedClient, nil, &stats,
		nil,
	)
	fedAPI := NewFederationInternalAPI(
		testDB, &cfg, nil, fedClient, &stats, nil, queues, nil,
	)

	req := api.PerformWakeupServersRequest{
		ServerNames: []spec.ServerName{server},
	}
	res := api.PerformWakeupServersResponse{}
	err = fedAPI.PerformWakeupServers(context.Background(), &req, &res)
	assert.NoError(t, err)

	blacklisted, err = testDB.IsServerBlacklisted(server)
	assert.NoError(t, err)
	assert.False(t, blacklisted)
	offline, err = testDB.IsServerAssumedOffline(context.Background(), server)
	assert.NoError(t, err)
	assert.False(t, offline)
}

func TestQueryRelayServers(t *testing.T) {
	testDB := test.NewInMemoryFederationDatabase()

	server := spec.ServerName("wakeup")
	relayServers := []spec.ServerName{"relay1", "relay2"}
	err := testDB.P2PAddRelayServersForServer(context.Background(), server, relayServers)
	assert.NoError(t, err)

	_, key, err := ed25519.GenerateKey(nil)
	assert.NoError(t, err)
	cfg := config.FederationAPI{
		Matrix: &config.Global{
			SigningIdentity: fclient.SigningIdentity{
				ServerName: "relay",
				KeyID:      "ed25519:1",
				PrivateKey: key,
			},
		},
	}
	fedClient := &testFedClient{}
	stats := statistics.NewStatistics(testDB, FailuresUntilBlacklist, FailuresUntilAssumedOffline)
	queues := queue.NewOutgoingQueues(
		testDB, process.NewProcessContext(),
		false,
		cfg.Matrix.ServerName, fedClient, nil, &stats,
		nil,
	)
	fedAPI := NewFederationInternalAPI(
		testDB, &cfg, nil, fedClient, &stats, nil, queues, nil,
	)

	req := api.P2PQueryRelayServersRequest{
		Server: server,
	}
	res := api.P2PQueryRelayServersResponse{}
	err = fedAPI.P2PQueryRelayServers(context.Background(), &req, &res)
	assert.NoError(t, err)

	assert.Equal(t, len(relayServers), len(res.RelayServers))
}

func TestRemoveRelayServers(t *testing.T) {
	testDB := test.NewInMemoryFederationDatabase()

	server := spec.ServerName("wakeup")
	relayServers := []spec.ServerName{"relay1", "relay2"}
	err := testDB.P2PAddRelayServersForServer(context.Background(), server, relayServers)
	assert.NoError(t, err)

	_, key, err := ed25519.GenerateKey(nil)
	assert.NoError(t, err)
	cfg := config.FederationAPI{
		Matrix: &config.Global{
			SigningIdentity: fclient.SigningIdentity{
				ServerName: "relay",
				KeyID:      "ed25519:1",
				PrivateKey: key,
			},
		},
	}
	fedClient := &testFedClient{}
	stats := statistics.NewStatistics(testDB, FailuresUntilBlacklist, FailuresUntilAssumedOffline)
	queues := queue.NewOutgoingQueues(
		testDB, process.NewProcessContext(),
		false,
		cfg.Matrix.ServerName, fedClient, nil, &stats,
		nil,
	)
	fedAPI := NewFederationInternalAPI(
		testDB, &cfg, nil, fedClient, &stats, nil, queues, nil,
	)

	req := api.P2PRemoveRelayServersRequest{
		Server:       server,
		RelayServers: []spec.ServerName{"relay1"},
	}
	res := api.P2PRemoveRelayServersResponse{}
	err = fedAPI.P2PRemoveRelayServers(context.Background(), &req, &res)
	assert.NoError(t, err)

	finalRelays, err := testDB.P2PGetRelayServersForServer(context.Background(), server)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(finalRelays))
	assert.Equal(t, spec.ServerName("relay2"), finalRelays[0])
}

func TestPerformDirectoryLookup(t *testing.T) {
	testDB := test.NewInMemoryFederationDatabase()

	_, key, err := ed25519.GenerateKey(nil)
	assert.NoError(t, err)
	cfg := config.FederationAPI{
		Matrix: &config.Global{
			SigningIdentity: fclient.SigningIdentity{
				ServerName: "relay",
				KeyID:      "ed25519:1",
				PrivateKey: key,
			},
		},
	}
	fedClient := &testFedClient{}
	stats := statistics.NewStatistics(testDB, FailuresUntilBlacklist, FailuresUntilAssumedOffline)
	queues := queue.NewOutgoingQueues(
		testDB, process.NewProcessContext(),
		false,
		cfg.Matrix.ServerName, fedClient, nil, &stats,
		nil,
	)
	fedAPI := NewFederationInternalAPI(
		testDB, &cfg, nil, fedClient, &stats, nil, queues, nil,
	)

	req := api.PerformDirectoryLookupRequest{
		RoomAlias:  "room",
		ServerName: "server",
	}
	res := api.PerformDirectoryLookupResponse{}
	err = fedAPI.PerformDirectoryLookup(context.Background(), &req, &res)
	assert.NoError(t, err)
}

func TestPerformDirectoryLookupRelaying(t *testing.T) {
	testDB := test.NewInMemoryFederationDatabase()

	server := spec.ServerName("wakeup")
	testDB.SetServerAssumedOffline(context.Background(), server)
	testDB.P2PAddRelayServersForServer(context.Background(), server, []spec.ServerName{"relay"})

	_, key, err := ed25519.GenerateKey(nil)
	assert.NoError(t, err)
	cfg := config.FederationAPI{
		Matrix: &config.Global{
			SigningIdentity: fclient.SigningIdentity{
				ServerName: "relay",
				KeyID:      "ed25519:1",
				PrivateKey: key,
			},
		},
	}
	fedClient := &testFedClient{}
	stats := statistics.NewStatistics(testDB, FailuresUntilBlacklist, FailuresUntilAssumedOffline)
	queues := queue.NewOutgoingQueues(
		testDB, process.NewProcessContext(),
		false,
		cfg.Matrix.ServerName, fedClient, nil, &stats,
		nil,
	)
	fedAPI := NewFederationInternalAPI(
		testDB, &cfg, nil, fedClient, &stats, nil, queues, nil,
	)

	req := api.PerformDirectoryLookupRequest{
		RoomAlias:  "room",
		ServerName: server,
	}
	res := api.PerformDirectoryLookupResponse{}
	err = fedAPI.PerformDirectoryLookup(context.Background(), &req, &res)
	assert.Error(t, err)
}
