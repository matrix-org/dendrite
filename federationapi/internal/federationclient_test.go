// Copyright 2024 New Vector Ltd.
// Copyright 2022 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package internal

import (
	"context"
	"fmt"
	"testing"

	"github.com/element-hq/dendrite/federationapi/queue"
	"github.com/element-hq/dendrite/federationapi/statistics"
	"github.com/element-hq/dendrite/setup/config"
	"github.com/element-hq/dendrite/setup/process"
	"github.com/element-hq/dendrite/test"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/stretchr/testify/assert"
)

const (
	FailuresUntilAssumedOffline = 3
	FailuresUntilBlacklist      = 8
)

func (t *testFedClient) QueryKeys(ctx context.Context, origin, s spec.ServerName, keys map[string][]string) (fclient.RespQueryKeys, error) {
	t.queryKeysCalled = true
	if t.shouldFail {
		return fclient.RespQueryKeys{}, fmt.Errorf("Failure")
	}
	return fclient.RespQueryKeys{}, nil
}

func (t *testFedClient) ClaimKeys(ctx context.Context, origin, s spec.ServerName, oneTimeKeys map[string]map[string]string) (fclient.RespClaimKeys, error) {
	t.claimKeysCalled = true
	if t.shouldFail {
		return fclient.RespClaimKeys{}, fmt.Errorf("Failure")
	}
	return fclient.RespClaimKeys{}, nil
}

func TestFederationClientQueryKeys(t *testing.T) {
	testDB := test.NewInMemoryFederationDatabase()

	cfg := config.FederationAPI{
		Matrix: &config.Global{
			SigningIdentity: fclient.SigningIdentity{
				ServerName: "server",
			},
		},
	}
	fedClient := &testFedClient{}
	stats := statistics.NewStatistics(testDB, FailuresUntilBlacklist, FailuresUntilAssumedOffline, false)
	queues := queue.NewOutgoingQueues(
		testDB, process.NewProcessContext(),
		false,
		cfg.Matrix.ServerName, fedClient, &stats,
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
			SigningIdentity: fclient.SigningIdentity{
				ServerName: "server",
			},
		},
	}
	fedClient := &testFedClient{}
	stats := statistics.NewStatistics(testDB, FailuresUntilBlacklist, FailuresUntilAssumedOffline, false)
	queues := queue.NewOutgoingQueues(
		testDB, process.NewProcessContext(),
		false,
		cfg.Matrix.ServerName, fedClient, &stats,
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
			SigningIdentity: fclient.SigningIdentity{
				ServerName: "server",
			},
		},
	}
	fedClient := &testFedClient{shouldFail: true}
	stats := statistics.NewStatistics(testDB, FailuresUntilBlacklist, FailuresUntilAssumedOffline, false)
	queues := queue.NewOutgoingQueues(
		testDB, process.NewProcessContext(),
		false,
		cfg.Matrix.ServerName, fedClient, &stats,
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
			SigningIdentity: fclient.SigningIdentity{
				ServerName: "server",
			},
		},
	}
	fedClient := &testFedClient{}
	stats := statistics.NewStatistics(testDB, FailuresUntilBlacklist, FailuresUntilAssumedOffline, false)
	queues := queue.NewOutgoingQueues(
		testDB, process.NewProcessContext(),
		false,
		cfg.Matrix.ServerName, fedClient, &stats,
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
			SigningIdentity: fclient.SigningIdentity{
				ServerName: "server",
			},
		},
	}
	fedClient := &testFedClient{}
	stats := statistics.NewStatistics(testDB, FailuresUntilBlacklist, FailuresUntilAssumedOffline, false)
	queues := queue.NewOutgoingQueues(
		testDB, process.NewProcessContext(),
		false,
		cfg.Matrix.ServerName, fedClient, &stats,
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
