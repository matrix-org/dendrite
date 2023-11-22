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

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/relayapi/storage/shared"
	"github.com/matrix-org/dendrite/test"
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
