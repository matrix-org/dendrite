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
	"fmt"
	"testing"

	"github.com/matrix-org/dendrite/federationapi/queue"
	"github.com/matrix-org/dendrite/federationapi/statistics"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/process"
	"github.com/matrix-org/dendrite/test"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/stretchr/testify/assert"
)

const (
	FailuresUntilAssumedOffline = 3
	FailuresUntilBlacklist      = 8
)

func (t *testFedClient) QueryKeys(ctx context.Context, origin, s gomatrixserverlib.ServerName, keys map[string][]string) (gomatrixserverlib.RespQueryKeys, error) {
	t.queryKeysCalled = true
	if t.shouldFail {
		return gomatrixserverlib.RespQueryKeys{}, fmt.Errorf("Failure")
	}
	return gomatrixserverlib.RespQueryKeys{}, nil
}

func (t *testFedClient) ClaimKeys(ctx context.Context, origin, s gomatrixserverlib.ServerName, oneTimeKeys map[string]map[string]string) (gomatrixserverlib.RespClaimKeys, error) {
	t.claimKeysCalled = true
	if t.shouldFail {
		return gomatrixserverlib.RespClaimKeys{}, fmt.Errorf("Failure")
	}
	return gomatrixserverlib.RespClaimKeys{}, nil
}

func TestFederationClientQueryKeys(t *testing.T) {
	testDB := test.NewInMemoryFederationDatabase()

	cfg := config.FederationAPI{
		Matrix: &config.Global{
			SigningIdentity: gomatrixserverlib.SigningIdentity{
				ServerName: "server",
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
	fedapi := FederationInternalAPI{
		db:         testDB,
		cfg:        &cfg,
		statistics: &stats,
		federation: fedClient,
		queues:     queues,
	}
	_, err := fedapi.QueryKeys(context.Background(), "origin", "server", nil)
	assert.Nil(t, err)
	assert.True(t, fedClient.queryKeysCalled)
}

func TestFederationClientQueryKeysBlacklisted(t *testing.T) {
	testDB := test.NewInMemoryFederationDatabase()
	testDB.AddServerToBlacklist("server")

	cfg := config.FederationAPI{
		Matrix: &config.Global{
			SigningIdentity: gomatrixserverlib.SigningIdentity{
				ServerName: "server",
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
	fedapi := FederationInternalAPI{
		db:         testDB,
		cfg:        &cfg,
		statistics: &stats,
		federation: fedClient,
		queues:     queues,
	}
	_, err := fedapi.QueryKeys(context.Background(), "origin", "server", nil)
	assert.NotNil(t, err)
	assert.False(t, fedClient.queryKeysCalled)
}

func TestFederationClientQueryKeysFailure(t *testing.T) {
	testDB := test.NewInMemoryFederationDatabase()

	cfg := config.FederationAPI{
		Matrix: &config.Global{
			SigningIdentity: gomatrixserverlib.SigningIdentity{
				ServerName: "server",
			},
		},
	}
	fedClient := &testFedClient{shouldFail: true}
	stats := statistics.NewStatistics(testDB, FailuresUntilBlacklist, FailuresUntilAssumedOffline)
	queues := queue.NewOutgoingQueues(
		testDB, process.NewProcessContext(),
		false,
		cfg.Matrix.ServerName, fedClient, nil, &stats,
		nil,
	)
	fedapi := FederationInternalAPI{
		db:         testDB,
		cfg:        &cfg,
		statistics: &stats,
		federation: fedClient,
		queues:     queues,
	}
	_, err := fedapi.QueryKeys(context.Background(), "origin", "server", nil)
	assert.NotNil(t, err)
	assert.True(t, fedClient.queryKeysCalled)
}

func TestFederationClientClaimKeys(t *testing.T) {
	testDB := test.NewInMemoryFederationDatabase()

	cfg := config.FederationAPI{
		Matrix: &config.Global{
			SigningIdentity: gomatrixserverlib.SigningIdentity{
				ServerName: "server",
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
	fedapi := FederationInternalAPI{
		db:         testDB,
		cfg:        &cfg,
		statistics: &stats,
		federation: fedClient,
		queues:     queues,
	}
	_, err := fedapi.ClaimKeys(context.Background(), "origin", "server", nil)
	assert.Nil(t, err)
	assert.True(t, fedClient.claimKeysCalled)
}

func TestFederationClientClaimKeysBlacklisted(t *testing.T) {
	testDB := test.NewInMemoryFederationDatabase()
	testDB.AddServerToBlacklist("server")

	cfg := config.FederationAPI{
		Matrix: &config.Global{
			SigningIdentity: gomatrixserverlib.SigningIdentity{
				ServerName: "server",
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
	fedapi := FederationInternalAPI{
		db:         testDB,
		cfg:        &cfg,
		statistics: &stats,
		federation: fedClient,
		queues:     queues,
	}
	_, err := fedapi.ClaimKeys(context.Background(), "origin", "server", nil)
	assert.NotNil(t, err)
	assert.False(t, fedClient.claimKeysCalled)
}
