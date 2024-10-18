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

	"github.com/element-hq/dendrite/internal/sqlutil"
	"github.com/element-hq/dendrite/relayapi/storage/shared"
	"github.com/element-hq/dendrite/test"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/stretchr/testify/assert"
)

type testFedClient struct {
	fclient.FederationClient
	shouldFail bool
	queryCount uint
	queueDepth uint
}

func (f *testFedClient) P2PGetTransactionFromRelay(
	ctx context.Context,
	u spec.UserID,
	prev fclient.RelayEntry,
	relayServer spec.ServerName,
) (res fclient.RespGetRelayTransaction, err error) {
	f.queryCount++
	if f.shouldFail {
		return res, fmt.Errorf("Error")
	}

	res = fclient.RespGetRelayTransaction{
		Transaction: gomatrixserverlib.Transaction{},
		EntryID:     0,
	}
	if f.queueDepth > 0 {
		res.EntriesQueued = true
	} else {
		res.EntriesQueued = false
	}
	f.queueDepth -= 1

	return
}

func TestPerformRelayServerSync(t *testing.T) {
	testDB := test.NewInMemoryRelayDatabase()
	db := shared.Database{
		Writer:         sqlutil.NewDummyWriter(),
		RelayQueue:     testDB,
		RelayQueueJSON: testDB,
	}

	userID, err := spec.NewUserID("@local:domain", false)
	assert.Nil(t, err, "Invalid userID")

	fedClient := &testFedClient{}
	relayAPI := NewRelayInternalAPI(
		&db, fedClient, nil, nil, nil, false, "", true,
	)

	err = relayAPI.PerformRelayServerSync(context.Background(), *userID, spec.ServerName("relay"))
	assert.NoError(t, err)
}

func TestPerformRelayServerSyncFedError(t *testing.T) {
	testDB := test.NewInMemoryRelayDatabase()
	db := shared.Database{
		Writer:         sqlutil.NewDummyWriter(),
		RelayQueue:     testDB,
		RelayQueueJSON: testDB,
	}

	userID, err := spec.NewUserID("@local:domain", false)
	assert.Nil(t, err, "Invalid userID")

	fedClient := &testFedClient{shouldFail: true}
	relayAPI := NewRelayInternalAPI(
		&db, fedClient, nil, nil, nil, false, "", true,
	)

	err = relayAPI.PerformRelayServerSync(context.Background(), *userID, spec.ServerName("relay"))
	assert.Error(t, err)
}

func TestPerformRelayServerSyncRunsUntilQueueEmpty(t *testing.T) {
	testDB := test.NewInMemoryRelayDatabase()
	db := shared.Database{
		Writer:         sqlutil.NewDummyWriter(),
		RelayQueue:     testDB,
		RelayQueueJSON: testDB,
	}

	userID, err := spec.NewUserID("@local:domain", false)
	assert.Nil(t, err, "Invalid userID")

	fedClient := &testFedClient{queueDepth: 2}
	relayAPI := NewRelayInternalAPI(
		&db, fedClient, nil, nil, nil, false, "", true,
	)

	err = relayAPI.PerformRelayServerSync(context.Background(), *userID, spec.ServerName("relay"))
	assert.NoError(t, err)
	assert.Equal(t, uint(3), fedClient.queryCount)
}
