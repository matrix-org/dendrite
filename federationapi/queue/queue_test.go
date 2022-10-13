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

package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"go.uber.org/atomic"
	"gotest.tools/v3/poll"

	"github.com/matrix-org/dendrite/federationapi/api"
	"github.com/matrix-org/dendrite/federationapi/statistics"
	"github.com/matrix-org/dendrite/federationapi/storage"
	"github.com/matrix-org/dendrite/federationapi/storage/shared"
	rsapi "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/process"
	"github.com/matrix-org/dendrite/test"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

var dbMutex sync.Mutex

type fakeDatabase struct {
	storage.Database
	pendingPDUServers  map[gomatrixserverlib.ServerName]struct{}
	pendingEDUServers  map[gomatrixserverlib.ServerName]struct{}
	blacklistedServers map[gomatrixserverlib.ServerName]struct{}
	pendingPDUs        map[*shared.Receipt]*gomatrixserverlib.HeaderedEvent
	pendingEDUs        map[*shared.Receipt]*gomatrixserverlib.EDU
	associatedPDUs     map[gomatrixserverlib.ServerName]map[*shared.Receipt]struct{}
	associatedEDUs     map[gomatrixserverlib.ServerName]map[*shared.Receipt]struct{}
}

func (d *fakeDatabase) StoreJSON(ctx context.Context, js string) (*shared.Receipt, error) {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	var event gomatrixserverlib.HeaderedEvent
	if err := json.Unmarshal([]byte(js), &event); err == nil {
		receipt := &shared.Receipt{}
		d.pendingPDUs[receipt] = &event
		return receipt, nil
	}

	var edu gomatrixserverlib.EDU
	if err := json.Unmarshal([]byte(js), &edu); err == nil {
		receipt := &shared.Receipt{}
		d.pendingEDUs[receipt] = &edu
		return receipt, nil
	}

	return nil, errors.New("Failed to determine type of json to store")
}

func (d *fakeDatabase) GetPendingPDUs(ctx context.Context, serverName gomatrixserverlib.ServerName, limit int) (pdus map[*shared.Receipt]*gomatrixserverlib.HeaderedEvent, err error) {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	pdus = make(map[*shared.Receipt]*gomatrixserverlib.HeaderedEvent)
	if receipts, ok := d.associatedPDUs[serverName]; ok {
		for receipt := range receipts {
			if event, ok := d.pendingPDUs[receipt]; ok {
				pdus[receipt] = event
			}
		}
	}
	return pdus, nil
}

func (d *fakeDatabase) GetPendingEDUs(ctx context.Context, serverName gomatrixserverlib.ServerName, limit int) (edus map[*shared.Receipt]*gomatrixserverlib.EDU, err error) {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	edus = make(map[*shared.Receipt]*gomatrixserverlib.EDU)
	if receipts, ok := d.associatedEDUs[serverName]; ok {
		for receipt := range receipts {
			if event, ok := d.pendingEDUs[receipt]; ok {
				edus[receipt] = event
			}
		}
	}
	return edus, nil
}

func (d *fakeDatabase) AssociatePDUWithDestination(ctx context.Context, transactionID gomatrixserverlib.TransactionID, serverName gomatrixserverlib.ServerName, receipt *shared.Receipt) error {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	if _, ok := d.pendingPDUs[receipt]; ok {
		if _, ok := d.associatedPDUs[serverName]; !ok {
			d.associatedPDUs[serverName] = make(map[*shared.Receipt]struct{})
		}
		d.associatedPDUs[serverName][receipt] = struct{}{}
		return nil
	} else {
		return errors.New("PDU doesn't exist")
	}
}

func (d *fakeDatabase) AssociateEDUWithDestination(ctx context.Context, serverName gomatrixserverlib.ServerName, receipt *shared.Receipt, eduType string, expireEDUTypes map[string]time.Duration) error {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	if _, ok := d.pendingEDUs[receipt]; ok {
		if _, ok := d.associatedEDUs[serverName]; !ok {
			d.associatedEDUs[serverName] = make(map[*shared.Receipt]struct{})
		}
		d.associatedEDUs[serverName][receipt] = struct{}{}
		return nil
	} else {
		return errors.New("EDU doesn't exist")
	}
}

func (d *fakeDatabase) CleanPDUs(ctx context.Context, serverName gomatrixserverlib.ServerName, receipts []*shared.Receipt) error {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	if pdus, ok := d.associatedPDUs[serverName]; ok {
		for _, receipt := range receipts {
			delete(pdus, receipt)
		}
	}

	return nil
}

func (d *fakeDatabase) CleanEDUs(ctx context.Context, serverName gomatrixserverlib.ServerName, receipts []*shared.Receipt) error {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	if edus, ok := d.associatedEDUs[serverName]; ok {
		for _, receipt := range receipts {
			delete(edus, receipt)
		}
	}

	return nil
}

func (d *fakeDatabase) GetPendingPDUCount(ctx context.Context, serverName gomatrixserverlib.ServerName) (int64, error) {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	var count int64
	if pdus, ok := d.associatedPDUs[serverName]; ok {
		count = int64(len(pdus))
	}
	return count, nil
}

func (d *fakeDatabase) GetPendingEDUCount(ctx context.Context, serverName gomatrixserverlib.ServerName) (int64, error) {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	var count int64
	if edus, ok := d.associatedEDUs[serverName]; ok {
		count = int64(len(edus))
	}
	return count, nil
}

func (d *fakeDatabase) GetPendingPDUServerNames(ctx context.Context) ([]gomatrixserverlib.ServerName, error) {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	servers := []gomatrixserverlib.ServerName{}
	for server := range d.pendingPDUServers {
		servers = append(servers, server)
	}
	return servers, nil
}

func (d *fakeDatabase) GetPendingEDUServerNames(ctx context.Context) ([]gomatrixserverlib.ServerName, error) {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	servers := []gomatrixserverlib.ServerName{}
	for server := range d.pendingEDUServers {
		servers = append(servers, server)
	}
	return servers, nil
}

func (d *fakeDatabase) AddServerToBlacklist(serverName gomatrixserverlib.ServerName) error {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	d.blacklistedServers[serverName] = struct{}{}
	return nil
}

func (d *fakeDatabase) RemoveServerFromBlacklist(serverName gomatrixserverlib.ServerName) error {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	delete(d.blacklistedServers, serverName)
	return nil
}

func (d *fakeDatabase) RemoveAllServersFromBlacklist() error {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	d.blacklistedServers = make(map[gomatrixserverlib.ServerName]struct{})
	return nil
}

func (d *fakeDatabase) IsServerBlacklisted(serverName gomatrixserverlib.ServerName) (bool, error) {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	isBlacklisted := false
	if _, ok := d.blacklistedServers[serverName]; ok {
		isBlacklisted = true
	}

	return isBlacklisted, nil
}

type stubFederationRoomServerAPI struct {
	rsapi.FederationRoomserverAPI
}

func (r *stubFederationRoomServerAPI) QueryServerBannedFromRoom(ctx context.Context, req *rsapi.QueryServerBannedFromRoomRequest, res *rsapi.QueryServerBannedFromRoomResponse) error {
	res.Banned = false
	return nil
}

type stubFederationClient struct {
	api.FederationClient
	shouldTxSucceed bool
	txCount         atomic.Uint32
}

func (f *stubFederationClient) SendTransaction(ctx context.Context, t gomatrixserverlib.Transaction) (res gomatrixserverlib.RespSend, err error) {
	var result error
	if !f.shouldTxSucceed {
		result = fmt.Errorf("transaction failed")
	}

	f.txCount.Add(1)
	return gomatrixserverlib.RespSend{}, result
}

func createDatabase() storage.Database {
	return &fakeDatabase{
		pendingPDUServers:  make(map[gomatrixserverlib.ServerName]struct{}),
		pendingEDUServers:  make(map[gomatrixserverlib.ServerName]struct{}),
		blacklistedServers: make(map[gomatrixserverlib.ServerName]struct{}),
		pendingPDUs:        make(map[*shared.Receipt]*gomatrixserverlib.HeaderedEvent),
		pendingEDUs:        make(map[*shared.Receipt]*gomatrixserverlib.EDU),
		associatedPDUs:     make(map[gomatrixserverlib.ServerName]map[*shared.Receipt]struct{}),
	}
}

func mustCreateEvent(t *testing.T) *gomatrixserverlib.HeaderedEvent {
	t.Helper()
	content := `{"type":"m.room.message"}`
	ev, err := gomatrixserverlib.NewEventFromTrustedJSON([]byte(content), false, gomatrixserverlib.RoomVersionV10)
	if err != nil {
		t.Fatalf("failed to create event: %v", err)
	}
	return ev.Headered(gomatrixserverlib.RoomVersionV10)
}

func testSetup(failuresUntilBlacklist uint32, shouldTxSucceed bool) (storage.Database, *stubFederationClient, *OutgoingQueues) {
	db := createDatabase()

	fc := &stubFederationClient{
		shouldTxSucceed: shouldTxSucceed,
		txCount:         *atomic.NewUint32(0),
	}
	rs := &stubFederationRoomServerAPI{}
	stats := &statistics.Statistics{
		DB:                     db,
		FailuresUntilBlacklist: failuresUntilBlacklist,
	}
	signingInfo := &SigningInfo{
		KeyID:      "ed25519:auto",
		PrivateKey: test.PrivateKeyA,
		ServerName: "localhost",
	}
	queues := NewOutgoingQueues(db, process.NewProcessContext(), false, "localhost", fc, rs, stats, signingInfo)

	return db, fc, queues
}

func TestSendTransactionOnSuccessRemovedFromDB(t *testing.T) {
	ctx := context.Background()
	failuresUntilBlacklist := uint32(16)
	destination := gomatrixserverlib.ServerName("remotehost")
	db, fc, queues := testSetup(failuresUntilBlacklist, true)

	ev := mustCreateEvent(t)
	err := queues.SendEvent(ev, "localhost", []gomatrixserverlib.ServerName{destination})
	assert.NoError(t, err)

	check := func(log poll.LogT) poll.Result {
		if fc.txCount.Load() >= 1 {
			data, err := db.GetPendingPDUs(ctx, destination, 100)
			assert.NoError(t, err)
			if len(data) == 0 {
				return poll.Success()
			}
			return poll.Continue("waiting for event to be removed from database")
		}
		return poll.Continue("waiting for more send attempts before checking database")
	}
	poll.WaitOn(t, check, poll.WithTimeout(5*time.Second), poll.WithDelay(100*time.Millisecond))
}

func TestSendTransactionOnFailStoredInDB(t *testing.T) {
	ctx := context.Background()
	failuresUntilBlacklist := uint32(16)
	destination := gomatrixserverlib.ServerName("remotehost")
	db, fc, queues := testSetup(failuresUntilBlacklist, false)

	ev := mustCreateEvent(t)
	err := queues.SendEvent(ev, "localhost", []gomatrixserverlib.ServerName{destination})
	assert.NoError(t, err)

	check := func(log poll.LogT) poll.Result {
		// Wait for 2 backoff attempts to ensure there was adequate time to attempt sending
		if fc.txCount.Load() >= 2 {
			data, err := db.GetPendingPDUs(ctx, destination, 100)
			assert.NoError(t, err)
			if len(data) == 1 {
				return poll.Success()
			}
			return poll.Continue("waiting for event to be added to database")
		}
		return poll.Continue("waiting for more send attempts before checking database")
	}
	poll.WaitOn(t, check, poll.WithTimeout(5*time.Second), poll.WithDelay(100*time.Millisecond))
}

func TestSendTransactionMultipleFailuresBlacklisted(t *testing.T) {
	ctx := context.Background()
	failuresUntilBlacklist := uint32(2)
	destination := gomatrixserverlib.ServerName("remotehost")
	db, fc, queues := testSetup(failuresUntilBlacklist, false)

	ev := mustCreateEvent(t)
	err := queues.SendEvent(ev, "localhost", []gomatrixserverlib.ServerName{destination})
	assert.NoError(t, err)

	check := func(log poll.LogT) poll.Result {
		if fc.txCount.Load() >= failuresUntilBlacklist {
			data, err := db.GetPendingPDUs(ctx, destination, 100)
			assert.NoError(t, err)
			if len(data) == 1 {
				if val, _ := db.IsServerBlacklisted(destination); val {
					return poll.Success()
				}
				return poll.Continue("waiting for server to be blacklisted")
			}
			return poll.Continue("waiting for event to be added to database")
		}
		return poll.Continue("waiting for more send attempts before checking database")
	}
	poll.WaitOn(t, check, poll.WithTimeout(5*time.Second), poll.WithDelay(100*time.Millisecond))
}

func TestRetryServerSendsSuccessfully(t *testing.T) {
	ctx := context.Background()
	failuresUntilBlacklist := uint32(1)
	destination := gomatrixserverlib.ServerName("remotehost")
	db, fc, queues := testSetup(failuresUntilBlacklist, false)

	ev := mustCreateEvent(t)
	err := queues.SendEvent(ev, "localhost", []gomatrixserverlib.ServerName{destination})
	assert.NoError(t, err)

	checkBlacklisted := func(log poll.LogT) poll.Result {
		if fc.txCount.Load() >= failuresUntilBlacklist {
			data, err := db.GetPendingPDUs(ctx, destination, 100)
			assert.NoError(t, err)
			if len(data) == 1 {
				if val, _ := db.IsServerBlacklisted(destination); val {
					return poll.Success()
				}
				return poll.Continue("waiting for server to be blacklisted")
			}
			return poll.Continue("waiting for event to be added to database")
		}
		return poll.Continue("waiting for more send attempts before checking database")
	}
	poll.WaitOn(t, checkBlacklisted, poll.WithTimeout(5*time.Second), poll.WithDelay(100*time.Millisecond))

	fc.shouldTxSucceed = true
	db.RemoveServerFromBlacklist(destination)
	queues.RetryServer(destination)
	checkRetry := func(log poll.LogT) poll.Result {
		data, err := db.GetPendingPDUs(ctx, destination, 100)
		assert.NoError(t, err)
		if len(data) == 0 {
			return poll.Success()
		}
		return poll.Continue("waiting for event to be removed from database")
	}
	poll.WaitOn(t, checkRetry, poll.WithTimeout(5*time.Second), poll.WithDelay(100*time.Millisecond))
}
