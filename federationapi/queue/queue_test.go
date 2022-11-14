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

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"

	"github.com/matrix-org/dendrite/federationapi/api"
	"github.com/matrix-org/dendrite/federationapi/statistics"
	"github.com/matrix-org/dendrite/federationapi/storage"
	"github.com/matrix-org/dendrite/federationapi/storage/shared"
	rsapi "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/process"
	"github.com/matrix-org/dendrite/test"
	"github.com/matrix-org/dendrite/test/testrig"
)

func mustCreateFederationDatabase(t *testing.T, dbType test.DBType, realDatabase bool) (storage.Database, *process.ProcessContext, func()) {
	if realDatabase {
		// Real Database/s
		b, baseClose := testrig.CreateBaseDendrite(t, dbType)
		connStr, dbClose := test.PrepareDBConnectionString(t, dbType)
		db, err := storage.NewDatabase(b, &config.DatabaseOptions{
			ConnectionString: config.DataSource(connStr),
		}, b.Caches, b.Cfg.Global.IsLocalServerName)
		if err != nil {
			t.Fatalf("NewDatabase returned %s", err)
		}
		return db, b.ProcessContext, func() {
			dbClose()
			baseClose()
		}
	} else {
		// Fake Database
		db := createDatabase()
		b := struct {
			ProcessContext *process.ProcessContext
		}{ProcessContext: process.NewProcessContext()}
		return db, b.ProcessContext, func() {}
	}
}

func createDatabase() storage.Database {
	return &fakeDatabase{
		pendingPDUServers:  make(map[gomatrixserverlib.ServerName]struct{}),
		pendingEDUServers:  make(map[gomatrixserverlib.ServerName]struct{}),
		blacklistedServers: make(map[gomatrixserverlib.ServerName]struct{}),
		pendingPDUs:        make(map[*shared.Receipt]*gomatrixserverlib.HeaderedEvent),
		pendingEDUs:        make(map[*shared.Receipt]*gomatrixserverlib.EDU),
		associatedPDUs:     make(map[gomatrixserverlib.ServerName]map[*shared.Receipt]struct{}),
		associatedEDUs:     make(map[gomatrixserverlib.ServerName]map[*shared.Receipt]struct{}),
	}
}

type fakeDatabase struct {
	storage.Database
	dbMutex            sync.Mutex
	pendingPDUServers  map[gomatrixserverlib.ServerName]struct{}
	pendingEDUServers  map[gomatrixserverlib.ServerName]struct{}
	blacklistedServers map[gomatrixserverlib.ServerName]struct{}
	pendingPDUs        map[*shared.Receipt]*gomatrixserverlib.HeaderedEvent
	pendingEDUs        map[*shared.Receipt]*gomatrixserverlib.EDU
	associatedPDUs     map[gomatrixserverlib.ServerName]map[*shared.Receipt]struct{}
	associatedEDUs     map[gomatrixserverlib.ServerName]map[*shared.Receipt]struct{}
}

var nidMutex sync.Mutex
var nid = int64(0)

func (d *fakeDatabase) StoreJSON(ctx context.Context, js string) (*shared.Receipt, error) {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	var event gomatrixserverlib.HeaderedEvent
	if err := json.Unmarshal([]byte(js), &event); err == nil {
		nidMutex.Lock()
		defer nidMutex.Unlock()
		nid++
		receipt := shared.NewReceipt(nid)
		d.pendingPDUs[&receipt] = &event
		return &receipt, nil
	}

	var edu gomatrixserverlib.EDU
	if err := json.Unmarshal([]byte(js), &edu); err == nil {
		nidMutex.Lock()
		defer nidMutex.Unlock()
		nid++
		receipt := shared.NewReceipt(nid)
		d.pendingEDUs[&receipt] = &edu
		return &receipt, nil
	}

	return nil, errors.New("Failed to determine type of json to store")
}

func (d *fakeDatabase) GetPendingPDUs(ctx context.Context, serverName gomatrixserverlib.ServerName, limit int) (pdus map[*shared.Receipt]*gomatrixserverlib.HeaderedEvent, err error) {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	pduCount := 0
	pdus = make(map[*shared.Receipt]*gomatrixserverlib.HeaderedEvent)
	if receipts, ok := d.associatedPDUs[serverName]; ok {
		for receipt := range receipts {
			if event, ok := d.pendingPDUs[receipt]; ok {
				pdus[receipt] = event
				pduCount++
				if pduCount == limit {
					break
				}
			}
		}
	}
	return pdus, nil
}

func (d *fakeDatabase) GetPendingEDUs(ctx context.Context, serverName gomatrixserverlib.ServerName, limit int) (edus map[*shared.Receipt]*gomatrixserverlib.EDU, err error) {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	eduCount := 0
	edus = make(map[*shared.Receipt]*gomatrixserverlib.EDU)
	if receipts, ok := d.associatedEDUs[serverName]; ok {
		for receipt := range receipts {
			if event, ok := d.pendingEDUs[receipt]; ok {
				edus[receipt] = event
				eduCount++
				if eduCount == limit {
					break
				}
			}
		}
	}
	return edus, nil
}

func (d *fakeDatabase) AssociatePDUWithDestinations(ctx context.Context, destinations map[gomatrixserverlib.ServerName]struct{}, receipt *shared.Receipt) error {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	if _, ok := d.pendingPDUs[receipt]; ok {
		for destination := range destinations {
			if _, ok := d.associatedPDUs[destination]; !ok {
				d.associatedPDUs[destination] = make(map[*shared.Receipt]struct{})
			}
			d.associatedPDUs[destination][receipt] = struct{}{}
		}

		return nil
	} else {
		return errors.New("PDU doesn't exist")
	}
}

func (d *fakeDatabase) AssociateEDUWithDestinations(ctx context.Context, destinations map[gomatrixserverlib.ServerName]struct{}, receipt *shared.Receipt, eduType string, expireEDUTypes map[string]time.Duration) error {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	if _, ok := d.pendingEDUs[receipt]; ok {
		for destination := range destinations {
			if _, ok := d.associatedEDUs[destination]; !ok {
				d.associatedEDUs[destination] = make(map[*shared.Receipt]struct{})
			}
			d.associatedEDUs[destination][receipt] = struct{}{}
		}

		return nil
	} else {
		return errors.New("EDU doesn't exist")
	}
}

func (d *fakeDatabase) CleanPDUs(ctx context.Context, serverName gomatrixserverlib.ServerName, receipts []*shared.Receipt) error {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	if pdus, ok := d.associatedPDUs[serverName]; ok {
		for _, receipt := range receipts {
			delete(pdus, receipt)
		}
	}

	return nil
}

func (d *fakeDatabase) CleanEDUs(ctx context.Context, serverName gomatrixserverlib.ServerName, receipts []*shared.Receipt) error {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	if edus, ok := d.associatedEDUs[serverName]; ok {
		for _, receipt := range receipts {
			delete(edus, receipt)
		}
	}

	return nil
}

func (d *fakeDatabase) GetPendingPDUCount(ctx context.Context, serverName gomatrixserverlib.ServerName) (int64, error) {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	var count int64
	if pdus, ok := d.associatedPDUs[serverName]; ok {
		count = int64(len(pdus))
	}
	return count, nil
}

func (d *fakeDatabase) GetPendingEDUCount(ctx context.Context, serverName gomatrixserverlib.ServerName) (int64, error) {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	var count int64
	if edus, ok := d.associatedEDUs[serverName]; ok {
		count = int64(len(edus))
	}
	return count, nil
}

func (d *fakeDatabase) GetPendingPDUServerNames(ctx context.Context) ([]gomatrixserverlib.ServerName, error) {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	servers := []gomatrixserverlib.ServerName{}
	for server := range d.pendingPDUServers {
		servers = append(servers, server)
	}
	return servers, nil
}

func (d *fakeDatabase) GetPendingEDUServerNames(ctx context.Context) ([]gomatrixserverlib.ServerName, error) {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	servers := []gomatrixserverlib.ServerName{}
	for server := range d.pendingEDUServers {
		servers = append(servers, server)
	}
	return servers, nil
}

func (d *fakeDatabase) AddServerToBlacklist(serverName gomatrixserverlib.ServerName) error {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	d.blacklistedServers[serverName] = struct{}{}
	return nil
}

func (d *fakeDatabase) RemoveServerFromBlacklist(serverName gomatrixserverlib.ServerName) error {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	delete(d.blacklistedServers, serverName)
	return nil
}

func (d *fakeDatabase) RemoveAllServersFromBlacklist() error {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	d.blacklistedServers = make(map[gomatrixserverlib.ServerName]struct{})
	return nil
}

func (d *fakeDatabase) IsServerBlacklisted(serverName gomatrixserverlib.ServerName) (bool, error) {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

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

func mustCreatePDU(t *testing.T) *gomatrixserverlib.HeaderedEvent {
	t.Helper()
	content := `{"type":"m.room.message"}`
	ev, err := gomatrixserverlib.NewEventFromTrustedJSON([]byte(content), false, gomatrixserverlib.RoomVersionV10)
	if err != nil {
		t.Fatalf("failed to create event: %v", err)
	}
	return ev.Headered(gomatrixserverlib.RoomVersionV10)
}

func mustCreateEDU(t *testing.T) *gomatrixserverlib.EDU {
	t.Helper()
	return &gomatrixserverlib.EDU{Type: gomatrixserverlib.MTyping}
}

func testSetup(failuresUntilBlacklist uint32, shouldTxSucceed bool, t *testing.T, dbType test.DBType, realDatabase bool) (storage.Database, *stubFederationClient, *OutgoingQueues, *process.ProcessContext, func()) {
	db, processContext, close := mustCreateFederationDatabase(t, dbType, realDatabase)

	fc := &stubFederationClient{
		shouldTxSucceed: shouldTxSucceed,
		txCount:         *atomic.NewUint32(0),
	}
	rs := &stubFederationRoomServerAPI{}
	stats := statistics.NewStatistics(db, failuresUntilBlacklist)
	signingInfo := []*gomatrixserverlib.SigningIdentity{
		{
			KeyID:      "ed21019:auto",
			PrivateKey: test.PrivateKeyA,
			ServerName: "localhost",
		},
	}
	queues := NewOutgoingQueues(db, processContext, false, "localhost", fc, rs, &stats, signingInfo)

	return db, fc, queues, processContext, close
}

func TestSendPDUOnSuccessRemovedFromDB(t *testing.T) {
	t.Parallel()
	failuresUntilBlacklist := uint32(16)
	destination := gomatrixserverlib.ServerName("remotehost")
	db, fc, queues, pc, close := testSetup(failuresUntilBlacklist, true, t, test.DBTypeSQLite, false)
	defer close()
	defer func() {
		pc.ShutdownDendrite()
		<-pc.WaitForShutdown()
	}()

	ev := mustCreatePDU(t)
	err := queues.SendEvent(ev, "localhost", []gomatrixserverlib.ServerName{destination})
	assert.NoError(t, err)

	check := func(log poll.LogT) poll.Result {
		if fc.txCount.Load() == 1 {
			data, dbErr := db.GetPendingPDUs(pc.Context(), destination, 100)
			assert.NoError(t, dbErr)
			if len(data) == 0 {
				return poll.Success()
			}
			return poll.Continue("waiting for event to be removed from database. Currently present PDU: %d", len(data))
		}
		return poll.Continue("waiting for more send attempts before checking database. Currently %d", fc.txCount.Load())
	}
	poll.WaitOn(t, check, poll.WithTimeout(5*time.Second), poll.WithDelay(100*time.Millisecond))
}

func TestSendEDUOnSuccessRemovedFromDB(t *testing.T) {
	t.Parallel()
	failuresUntilBlacklist := uint32(16)
	destination := gomatrixserverlib.ServerName("remotehost")
	db, fc, queues, pc, close := testSetup(failuresUntilBlacklist, true, t, test.DBTypeSQLite, false)
	defer close()
	defer func() {
		pc.ShutdownDendrite()
		<-pc.WaitForShutdown()
	}()

	ev := mustCreateEDU(t)
	err := queues.SendEDU(ev, "localhost", []gomatrixserverlib.ServerName{destination})
	assert.NoError(t, err)

	check := func(log poll.LogT) poll.Result {
		if fc.txCount.Load() == 1 {
			data, dbErr := db.GetPendingEDUs(pc.Context(), destination, 100)
			assert.NoError(t, dbErr)
			if len(data) == 0 {
				return poll.Success()
			}
			return poll.Continue("waiting for event to be removed from database. Currently present EDU: %d", len(data))
		}
		return poll.Continue("waiting for more send attempts before checking database. Currently %d", fc.txCount.Load())
	}
	poll.WaitOn(t, check, poll.WithTimeout(5*time.Second), poll.WithDelay(100*time.Millisecond))
}

func TestSendPDUOnFailStoredInDB(t *testing.T) {
	t.Parallel()
	failuresUntilBlacklist := uint32(16)
	destination := gomatrixserverlib.ServerName("remotehost")
	db, fc, queues, pc, close := testSetup(failuresUntilBlacklist, false, t, test.DBTypeSQLite, false)
	defer close()
	defer func() {
		pc.ShutdownDendrite()
		<-pc.WaitForShutdown()
	}()

	ev := mustCreatePDU(t)
	err := queues.SendEvent(ev, "localhost", []gomatrixserverlib.ServerName{destination})
	assert.NoError(t, err)

	check := func(log poll.LogT) poll.Result {
		// Wait for 2 backoff attempts to ensure there was adequate time to attempt sending
		if fc.txCount.Load() >= 2 {
			data, dbErr := db.GetPendingPDUs(pc.Context(), destination, 100)
			assert.NoError(t, dbErr)
			if len(data) == 1 {
				return poll.Success()
			}
			return poll.Continue("waiting for event to be added to database. Currently present PDU: %d", len(data))
		}
		return poll.Continue("waiting for more send attempts before checking database. Currently %d", fc.txCount.Load())
	}
	poll.WaitOn(t, check, poll.WithTimeout(5*time.Second), poll.WithDelay(100*time.Millisecond))
}

func TestSendEDUOnFailStoredInDB(t *testing.T) {
	t.Parallel()
	failuresUntilBlacklist := uint32(16)
	destination := gomatrixserverlib.ServerName("remotehost")
	db, fc, queues, pc, close := testSetup(failuresUntilBlacklist, false, t, test.DBTypeSQLite, false)
	defer close()
	defer func() {
		pc.ShutdownDendrite()
		<-pc.WaitForShutdown()
	}()

	ev := mustCreateEDU(t)
	err := queues.SendEDU(ev, "localhost", []gomatrixserverlib.ServerName{destination})
	assert.NoError(t, err)

	check := func(log poll.LogT) poll.Result {
		// Wait for 2 backoff attempts to ensure there was adequate time to attempt sending
		if fc.txCount.Load() >= 2 {
			data, dbErr := db.GetPendingEDUs(pc.Context(), destination, 100)
			assert.NoError(t, dbErr)
			if len(data) == 1 {
				return poll.Success()
			}
			return poll.Continue("waiting for event to be added to database. Currently present EDU: %d", len(data))
		}
		return poll.Continue("waiting for more send attempts before checking database. Currently %d", fc.txCount.Load())
	}
	poll.WaitOn(t, check, poll.WithTimeout(5*time.Second), poll.WithDelay(100*time.Millisecond))
}

func TestSendPDUAgainDoesntInterruptBackoff(t *testing.T) {
	t.Parallel()
	failuresUntilBlacklist := uint32(16)
	destination := gomatrixserverlib.ServerName("remotehost")
	db, fc, queues, pc, close := testSetup(failuresUntilBlacklist, false, t, test.DBTypeSQLite, false)
	defer close()
	defer func() {
		pc.ShutdownDendrite()
		<-pc.WaitForShutdown()
	}()

	ev := mustCreatePDU(t)
	err := queues.SendEvent(ev, "localhost", []gomatrixserverlib.ServerName{destination})
	assert.NoError(t, err)

	check := func(log poll.LogT) poll.Result {
		// Wait for 2 backoff attempts to ensure there was adequate time to attempt sending
		if fc.txCount.Load() >= 2 {
			data, dbErr := db.GetPendingPDUs(pc.Context(), destination, 100)
			assert.NoError(t, dbErr)
			if len(data) == 1 {
				return poll.Success()
			}
			return poll.Continue("waiting for event to be added to database. Currently present PDU: %d", len(data))
		}
		return poll.Continue("waiting for more send attempts before checking database. Currently %d", fc.txCount.Load())
	}
	poll.WaitOn(t, check, poll.WithTimeout(5*time.Second), poll.WithDelay(100*time.Millisecond))

	fc.shouldTxSucceed = true
	ev = mustCreatePDU(t)
	err = queues.SendEvent(ev, "localhost", []gomatrixserverlib.ServerName{destination})
	assert.NoError(t, err)

	pollEnd := time.Now().Add(1 * time.Second)
	immediateCheck := func(log poll.LogT) poll.Result {
		data, dbErr := db.GetPendingPDUs(pc.Context(), destination, 100)
		assert.NoError(t, dbErr)
		if len(data) == 0 {
			return poll.Error(fmt.Errorf("The backoff was interrupted early"))
		}
		if time.Now().After(pollEnd) {
			// Allow more than enough time for the backoff to be interrupted before
			// reporting that it wasn't.
			return poll.Success()
		}
		return poll.Continue("waiting for events to be removed from database. Currently present PDU: %d", len(data))
	}
	poll.WaitOn(t, immediateCheck, poll.WithTimeout(2*time.Second), poll.WithDelay(100*time.Millisecond))
}

func TestSendEDUAgainDoesntInterruptBackoff(t *testing.T) {
	t.Parallel()
	failuresUntilBlacklist := uint32(16)
	destination := gomatrixserverlib.ServerName("remotehost")
	db, fc, queues, pc, close := testSetup(failuresUntilBlacklist, false, t, test.DBTypeSQLite, false)
	defer close()
	defer func() {
		pc.ShutdownDendrite()
		<-pc.WaitForShutdown()
	}()

	ev := mustCreateEDU(t)
	err := queues.SendEDU(ev, "localhost", []gomatrixserverlib.ServerName{destination})
	assert.NoError(t, err)

	check := func(log poll.LogT) poll.Result {
		// Wait for 2 backoff attempts to ensure there was adequate time to attempt sending
		if fc.txCount.Load() >= 2 {
			data, dbErr := db.GetPendingEDUs(pc.Context(), destination, 100)
			assert.NoError(t, dbErr)
			if len(data) == 1 {
				return poll.Success()
			}
			return poll.Continue("waiting for event to be added to database. Currently present EDU: %d", len(data))
		}
		return poll.Continue("waiting for more send attempts before checking database. Currently %d", fc.txCount.Load())
	}
	poll.WaitOn(t, check, poll.WithTimeout(5*time.Second), poll.WithDelay(100*time.Millisecond))

	fc.shouldTxSucceed = true
	ev = mustCreateEDU(t)
	err = queues.SendEDU(ev, "localhost", []gomatrixserverlib.ServerName{destination})
	assert.NoError(t, err)

	pollEnd := time.Now().Add(1 * time.Second)
	immediateCheck := func(log poll.LogT) poll.Result {
		data, dbErr := db.GetPendingEDUs(pc.Context(), destination, 100)
		assert.NoError(t, dbErr)
		if len(data) == 0 {
			return poll.Error(fmt.Errorf("The backoff was interrupted early"))
		}
		if time.Now().After(pollEnd) {
			// Allow more than enough time for the backoff to be interrupted before
			// reporting that it wasn't.
			return poll.Success()
		}
		return poll.Continue("waiting for events to be removed from database. Currently present EDU: %d", len(data))
	}
	poll.WaitOn(t, immediateCheck, poll.WithTimeout(2*time.Second), poll.WithDelay(100*time.Millisecond))
}

func TestSendPDUMultipleFailuresBlacklisted(t *testing.T) {
	t.Parallel()
	failuresUntilBlacklist := uint32(2)
	destination := gomatrixserverlib.ServerName("remotehost")
	db, fc, queues, pc, close := testSetup(failuresUntilBlacklist, false, t, test.DBTypeSQLite, false)
	defer close()
	defer func() {
		pc.ShutdownDendrite()
		<-pc.WaitForShutdown()
	}()

	ev := mustCreatePDU(t)
	err := queues.SendEvent(ev, "localhost", []gomatrixserverlib.ServerName{destination})
	assert.NoError(t, err)

	check := func(log poll.LogT) poll.Result {
		if fc.txCount.Load() == failuresUntilBlacklist {
			data, dbErr := db.GetPendingPDUs(pc.Context(), destination, 100)
			assert.NoError(t, dbErr)
			if len(data) == 1 {
				if val, _ := db.IsServerBlacklisted(destination); val {
					return poll.Success()
				}
				return poll.Continue("waiting for server to be blacklisted")
			}
			return poll.Continue("waiting for event to be added to database. Currently present PDU: %d", len(data))
		}
		return poll.Continue("waiting for more send attempts before checking database. Currently %d", fc.txCount.Load())
	}
	poll.WaitOn(t, check, poll.WithTimeout(5*time.Second), poll.WithDelay(100*time.Millisecond))
}

func TestSendEDUMultipleFailuresBlacklisted(t *testing.T) {
	t.Parallel()
	failuresUntilBlacklist := uint32(2)
	destination := gomatrixserverlib.ServerName("remotehost")
	db, fc, queues, pc, close := testSetup(failuresUntilBlacklist, false, t, test.DBTypeSQLite, false)
	defer close()
	defer func() {
		pc.ShutdownDendrite()
		<-pc.WaitForShutdown()
	}()

	ev := mustCreateEDU(t)
	err := queues.SendEDU(ev, "localhost", []gomatrixserverlib.ServerName{destination})
	assert.NoError(t, err)

	check := func(log poll.LogT) poll.Result {
		if fc.txCount.Load() == failuresUntilBlacklist {
			data, dbErr := db.GetPendingEDUs(pc.Context(), destination, 100)
			assert.NoError(t, dbErr)
			if len(data) == 1 {
				if val, _ := db.IsServerBlacklisted(destination); val {
					return poll.Success()
				}
				return poll.Continue("waiting for server to be blacklisted")
			}
			return poll.Continue("waiting for event to be added to database. Currently present EDU: %d", len(data))
		}
		return poll.Continue("waiting for more send attempts before checking database. Currently %d", fc.txCount.Load())
	}
	poll.WaitOn(t, check, poll.WithTimeout(5*time.Second), poll.WithDelay(100*time.Millisecond))
}

func TestSendPDUBlacklistedWithPriorExternalFailure(t *testing.T) {
	t.Parallel()
	failuresUntilBlacklist := uint32(2)
	destination := gomatrixserverlib.ServerName("remotehost")
	db, fc, queues, pc, close := testSetup(failuresUntilBlacklist, false, t, test.DBTypeSQLite, false)
	defer close()
	defer func() {
		pc.ShutdownDendrite()
		<-pc.WaitForShutdown()
	}()

	queues.statistics.ForServer(destination).Failure()

	ev := mustCreatePDU(t)
	err := queues.SendEvent(ev, "localhost", []gomatrixserverlib.ServerName{destination})
	assert.NoError(t, err)

	check := func(log poll.LogT) poll.Result {
		if fc.txCount.Load() == failuresUntilBlacklist {
			data, dbErr := db.GetPendingPDUs(pc.Context(), destination, 100)
			assert.NoError(t, dbErr)
			if len(data) == 1 {
				if val, _ := db.IsServerBlacklisted(destination); val {
					return poll.Success()
				}
				return poll.Continue("waiting for server to be blacklisted")
			}
			return poll.Continue("waiting for event to be added to database. Currently present PDU: %d", len(data))
		}
		return poll.Continue("waiting for more send attempts before checking database. Currently %d", fc.txCount.Load())
	}
	poll.WaitOn(t, check, poll.WithTimeout(5*time.Second), poll.WithDelay(100*time.Millisecond))
}

func TestSendEDUBlacklistedWithPriorExternalFailure(t *testing.T) {
	t.Parallel()
	failuresUntilBlacklist := uint32(2)
	destination := gomatrixserverlib.ServerName("remotehost")
	db, fc, queues, pc, close := testSetup(failuresUntilBlacklist, false, t, test.DBTypeSQLite, false)
	defer close()
	defer func() {
		pc.ShutdownDendrite()
		<-pc.WaitForShutdown()
	}()

	queues.statistics.ForServer(destination).Failure()

	ev := mustCreateEDU(t)
	err := queues.SendEDU(ev, "localhost", []gomatrixserverlib.ServerName{destination})
	assert.NoError(t, err)

	check := func(log poll.LogT) poll.Result {
		if fc.txCount.Load() == failuresUntilBlacklist {
			data, dbErr := db.GetPendingEDUs(pc.Context(), destination, 100)
			assert.NoError(t, dbErr)
			if len(data) == 1 {
				if val, _ := db.IsServerBlacklisted(destination); val {
					return poll.Success()
				}
				return poll.Continue("waiting for server to be blacklisted")
			}
			return poll.Continue("waiting for event to be added to database. Currently present EDU: %d", len(data))
		}
		return poll.Continue("waiting for more send attempts before checking database. Currently %d", fc.txCount.Load())
	}
	poll.WaitOn(t, check, poll.WithTimeout(5*time.Second), poll.WithDelay(100*time.Millisecond))
}

func TestRetryServerSendsPDUSuccessfully(t *testing.T) {
	t.Parallel()
	failuresUntilBlacklist := uint32(1)
	destination := gomatrixserverlib.ServerName("remotehost")
	db, fc, queues, pc, close := testSetup(failuresUntilBlacklist, false, t, test.DBTypeSQLite, false)
	defer close()
	defer func() {
		pc.ShutdownDendrite()
		<-pc.WaitForShutdown()
	}()

	// NOTE : getQueue before sending event to ensure we grab the same queue reference
	// before it is blacklisted and deleted.
	dest := queues.getQueue(destination)
	ev := mustCreatePDU(t)
	err := queues.SendEvent(ev, "localhost", []gomatrixserverlib.ServerName{destination})
	assert.NoError(t, err)

	checkBlacklisted := func(log poll.LogT) poll.Result {
		if fc.txCount.Load() == failuresUntilBlacklist {
			data, dbErr := db.GetPendingPDUs(pc.Context(), destination, 100)
			assert.NoError(t, dbErr)
			if len(data) == 1 {
				if val, _ := db.IsServerBlacklisted(destination); val {
					if !dest.running.Load() {
						return poll.Success()
					}
					return poll.Continue("waiting for queue to stop completely")
				}
				return poll.Continue("waiting for server to be blacklisted")
			}
			return poll.Continue("waiting for event to be added to database. Currently present PDU: %d", len(data))
		}
		return poll.Continue("waiting for more send attempts before checking database. Currently %d", fc.txCount.Load())
	}
	poll.WaitOn(t, checkBlacklisted, poll.WithTimeout(5*time.Second), poll.WithDelay(100*time.Millisecond))

	fc.shouldTxSucceed = true
	db.RemoveServerFromBlacklist(destination)
	queues.RetryServer(destination)
	checkRetry := func(log poll.LogT) poll.Result {
		data, dbErr := db.GetPendingPDUs(pc.Context(), destination, 100)
		assert.NoError(t, dbErr)
		if len(data) == 0 {
			return poll.Success()
		}
		return poll.Continue("waiting for event to be removed from database. Currently present PDU: %d", len(data))
	}
	poll.WaitOn(t, checkRetry, poll.WithTimeout(5*time.Second), poll.WithDelay(100*time.Millisecond))
}

func TestRetryServerSendsEDUSuccessfully(t *testing.T) {
	t.Parallel()
	failuresUntilBlacklist := uint32(1)
	destination := gomatrixserverlib.ServerName("remotehost")
	db, fc, queues, pc, close := testSetup(failuresUntilBlacklist, false, t, test.DBTypeSQLite, false)
	defer close()
	defer func() {
		pc.ShutdownDendrite()
		<-pc.WaitForShutdown()
	}()

	// NOTE : getQueue before sending event to ensure we grab the same queue reference
	// before it is blacklisted and deleted.
	dest := queues.getQueue(destination)
	ev := mustCreateEDU(t)
	err := queues.SendEDU(ev, "localhost", []gomatrixserverlib.ServerName{destination})
	assert.NoError(t, err)

	checkBlacklisted := func(log poll.LogT) poll.Result {
		if fc.txCount.Load() == failuresUntilBlacklist {
			data, dbErr := db.GetPendingEDUs(pc.Context(), destination, 100)
			assert.NoError(t, dbErr)
			if len(data) == 1 {
				if val, _ := db.IsServerBlacklisted(destination); val {
					if !dest.running.Load() {
						return poll.Success()
					}
					return poll.Continue("waiting for queue to stop completely")
				}
				return poll.Continue("waiting for server to be blacklisted")
			}
			return poll.Continue("waiting for event to be added to database. Currently present EDU: %d", len(data))
		}
		return poll.Continue("waiting for more send attempts before checking database. Currently %d", fc.txCount.Load())
	}
	poll.WaitOn(t, checkBlacklisted, poll.WithTimeout(5*time.Second), poll.WithDelay(100*time.Millisecond))

	fc.shouldTxSucceed = true
	db.RemoveServerFromBlacklist(destination)
	queues.RetryServer(destination)
	checkRetry := func(log poll.LogT) poll.Result {
		data, dbErr := db.GetPendingEDUs(pc.Context(), destination, 100)
		assert.NoError(t, dbErr)
		if len(data) == 0 {
			return poll.Success()
		}
		return poll.Continue("waiting for event to be removed from database. Currently present EDU: %d", len(data))
	}
	poll.WaitOn(t, checkRetry, poll.WithTimeout(5*time.Second), poll.WithDelay(100*time.Millisecond))
}

func TestSendPDUBatches(t *testing.T) {
	t.Parallel()
	failuresUntilBlacklist := uint32(16)
	destination := gomatrixserverlib.ServerName("remotehost")

	// test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
	// db, fc, queues, pc, close := testSetup(failuresUntilBlacklist, true, t, dbType, true)
	db, fc, queues, pc, close := testSetup(failuresUntilBlacklist, true, t, test.DBTypeSQLite, false)
	defer close()
	defer func() {
		pc.ShutdownDendrite()
		<-pc.WaitForShutdown()
	}()

	destinations := map[gomatrixserverlib.ServerName]struct{}{destination: {}}
	// Populate database with > maxPDUsPerTransaction
	pduMultiplier := uint32(3)
	for i := 0; i < maxPDUsPerTransaction*int(pduMultiplier); i++ {
		ev := mustCreatePDU(t)
		headeredJSON, _ := json.Marshal(ev)
		nid, _ := db.StoreJSON(pc.Context(), string(headeredJSON))
		err := db.AssociatePDUWithDestinations(pc.Context(), destinations, nid)
		assert.NoError(t, err, "failed to associate PDU with destinations")
	}

	ev := mustCreatePDU(t)
	err := queues.SendEvent(ev, "localhost", []gomatrixserverlib.ServerName{destination})
	assert.NoError(t, err)

	check := func(log poll.LogT) poll.Result {
		if fc.txCount.Load() == pduMultiplier+1 { // +1 for the extra SendEvent()
			data, dbErr := db.GetPendingPDUs(pc.Context(), destination, 200)
			assert.NoError(t, dbErr)
			if len(data) == 0 {
				return poll.Success()
			}
			return poll.Continue("waiting for all events to be removed from database. Currently present PDU: %d", len(data))
		}
		return poll.Continue("waiting for the right amount of send attempts before checking database. Currently %d", fc.txCount.Load())
	}
	poll.WaitOn(t, check, poll.WithTimeout(5*time.Second), poll.WithDelay(100*time.Millisecond))
	// })
}

func TestSendEDUBatches(t *testing.T) {
	t.Parallel()
	failuresUntilBlacklist := uint32(16)
	destination := gomatrixserverlib.ServerName("remotehost")

	// test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
	// db, fc, queues, pc, close := testSetup(failuresUntilBlacklist, true, t, dbType, true)
	db, fc, queues, pc, close := testSetup(failuresUntilBlacklist, true, t, test.DBTypeSQLite, false)
	defer close()
	defer func() {
		pc.ShutdownDendrite()
		<-pc.WaitForShutdown()
	}()

	destinations := map[gomatrixserverlib.ServerName]struct{}{destination: {}}
	// Populate database with > maxEDUsPerTransaction
	eduMultiplier := uint32(3)
	for i := 0; i < maxEDUsPerTransaction*int(eduMultiplier); i++ {
		ev := mustCreateEDU(t)
		ephemeralJSON, _ := json.Marshal(ev)
		nid, _ := db.StoreJSON(pc.Context(), string(ephemeralJSON))
		err := db.AssociateEDUWithDestinations(pc.Context(), destinations, nid, ev.Type, nil)
		assert.NoError(t, err, "failed to associate EDU with destinations")
	}

	ev := mustCreateEDU(t)
	err := queues.SendEDU(ev, "localhost", []gomatrixserverlib.ServerName{destination})
	assert.NoError(t, err)

	check := func(log poll.LogT) poll.Result {
		if fc.txCount.Load() == eduMultiplier+1 { // +1 for the extra SendEvent()
			data, dbErr := db.GetPendingEDUs(pc.Context(), destination, 200)
			assert.NoError(t, dbErr)
			if len(data) == 0 {
				return poll.Success()
			}
			return poll.Continue("waiting for all events to be removed from database. Currently present EDU: %d", len(data))
		}
		return poll.Continue("waiting for the right amount of send attempts before checking database. Currently %d", fc.txCount.Load())
	}
	poll.WaitOn(t, check, poll.WithTimeout(5*time.Second), poll.WithDelay(100*time.Millisecond))
	// })
}

func TestSendPDUAndEDUBatches(t *testing.T) {
	t.Parallel()
	failuresUntilBlacklist := uint32(16)
	destination := gomatrixserverlib.ServerName("remotehost")

	// test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
	// db, fc, queues, pc, close := testSetup(failuresUntilBlacklist, true, t, dbType, true)
	db, fc, queues, pc, close := testSetup(failuresUntilBlacklist, true, t, test.DBTypeSQLite, false)
	defer close()
	defer func() {
		pc.ShutdownDendrite()
		<-pc.WaitForShutdown()
	}()

	destinations := map[gomatrixserverlib.ServerName]struct{}{destination: {}}
	// Populate database with > maxEDUsPerTransaction
	multiplier := uint32(3)
	for i := 0; i < maxPDUsPerTransaction*int(multiplier)+1; i++ {
		ev := mustCreatePDU(t)
		headeredJSON, _ := json.Marshal(ev)
		nid, _ := db.StoreJSON(pc.Context(), string(headeredJSON))
		err := db.AssociatePDUWithDestinations(pc.Context(), destinations, nid)
		assert.NoError(t, err, "failed to associate PDU with destinations")
	}

	for i := 0; i < maxEDUsPerTransaction*int(multiplier); i++ {
		ev := mustCreateEDU(t)
		ephemeralJSON, _ := json.Marshal(ev)
		nid, _ := db.StoreJSON(pc.Context(), string(ephemeralJSON))
		err := db.AssociateEDUWithDestinations(pc.Context(), destinations, nid, ev.Type, nil)
		assert.NoError(t, err, "failed to associate EDU with destinations")
	}

	ev := mustCreateEDU(t)
	err := queues.SendEDU(ev, "localhost", []gomatrixserverlib.ServerName{destination})
	assert.NoError(t, err)

	check := func(log poll.LogT) poll.Result {
		if fc.txCount.Load() == multiplier+1 { // +1 for the extra SendEvent()
			pduData, dbErrPDU := db.GetPendingPDUs(pc.Context(), destination, 200)
			assert.NoError(t, dbErrPDU)
			eduData, dbErrEDU := db.GetPendingEDUs(pc.Context(), destination, 200)
			assert.NoError(t, dbErrEDU)
			if len(pduData) == 0 && len(eduData) == 0 {
				return poll.Success()
			}
			return poll.Continue("waiting for all events to be removed from database. Currently present PDU: %d EDU: %d", len(pduData), len(eduData))
		}
		return poll.Continue("waiting for the right amount of send attempts before checking database. Currently %d", fc.txCount.Load())
	}
	poll.WaitOn(t, check, poll.WithTimeout(5*time.Second), poll.WithDelay(100*time.Millisecond))
	// })
}

func TestExternalFailureBackoffDoesntStartQueue(t *testing.T) {
	t.Parallel()
	failuresUntilBlacklist := uint32(16)
	destination := gomatrixserverlib.ServerName("remotehost")
	db, fc, queues, pc, close := testSetup(failuresUntilBlacklist, true, t, test.DBTypeSQLite, false)
	defer close()
	defer func() {
		pc.ShutdownDendrite()
		<-pc.WaitForShutdown()
	}()

	dest := queues.getQueue(destination)
	queues.statistics.ForServer(destination).Failure()
	destinations := map[gomatrixserverlib.ServerName]struct{}{destination: {}}
	ev := mustCreatePDU(t)
	headeredJSON, _ := json.Marshal(ev)
	nid, _ := db.StoreJSON(pc.Context(), string(headeredJSON))
	err := db.AssociatePDUWithDestinations(pc.Context(), destinations, nid)
	assert.NoError(t, err, "failed to associate PDU with destinations")

	pollEnd := time.Now().Add(3 * time.Second)
	runningCheck := func(log poll.LogT) poll.Result {
		if dest.running.Load() || fc.txCount.Load() > 0 {
			return poll.Error(fmt.Errorf("The queue was started"))
		}
		if time.Now().After(pollEnd) {
			// Allow more than enough time for the queue to be started in the case
			// of backoff triggering it to start.
			return poll.Success()
		}
		return poll.Continue("waiting to ensure queue doesn't start.")
	}
	poll.WaitOn(t, runningCheck, poll.WithTimeout(4*time.Second), poll.WithDelay(100*time.Millisecond))
}

func TestQueueInteractsWithRealDatabasePDUAndEDU(t *testing.T) {
	// NOTE : Only one test case against real databases can be run at a time.
	t.Parallel()
	failuresUntilBlacklist := uint32(1)
	destination := gomatrixserverlib.ServerName("remotehost")
	destinations := map[gomatrixserverlib.ServerName]struct{}{destination: {}}
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, fc, queues, pc, close := testSetup(failuresUntilBlacklist, false, t, dbType, true)
		// NOTE : These defers aren't called if go test is killed so the dbs may not get cleaned up.
		defer close()
		defer func() {
			pc.ShutdownDendrite()
			<-pc.WaitForShutdown()
		}()

		// NOTE : getQueue before sending event to ensure we grab the same queue reference
		// before it is blacklisted and deleted.
		dest := queues.getQueue(destination)
		ev := mustCreatePDU(t)
		err := queues.SendEvent(ev, "localhost", []gomatrixserverlib.ServerName{destination})
		assert.NoError(t, err)

		// NOTE : The server can be blacklisted before this, so manually inject the event
		// into the database.
		edu := mustCreateEDU(t)
		ephemeralJSON, _ := json.Marshal(edu)
		nid, _ := db.StoreJSON(pc.Context(), string(ephemeralJSON))
		err = db.AssociateEDUWithDestinations(pc.Context(), destinations, nid, edu.Type, nil)
		assert.NoError(t, err, "failed to associate EDU with destinations")

		checkBlacklisted := func(log poll.LogT) poll.Result {
			if fc.txCount.Load() == failuresUntilBlacklist {
				pduData, dbErrPDU := db.GetPendingPDUs(pc.Context(), destination, 200)
				assert.NoError(t, dbErrPDU)
				eduData, dbErrEDU := db.GetPendingEDUs(pc.Context(), destination, 200)
				assert.NoError(t, dbErrEDU)
				if len(pduData) == 1 && len(eduData) == 1 {
					if val, _ := db.IsServerBlacklisted(destination); val {
						if !dest.running.Load() {
							return poll.Success()
						}
						return poll.Continue("waiting for queue to stop completely")
					}
					return poll.Continue("waiting for server to be blacklisted")
				}
				return poll.Continue("waiting for events to be added to database. Currently present PDU: %d EDU: %d", len(pduData), len(eduData))
			}
			return poll.Continue("waiting for more send attempts before checking database. Currently %d", fc.txCount.Load())
		}
		poll.WaitOn(t, checkBlacklisted, poll.WithTimeout(10*time.Second), poll.WithDelay(100*time.Millisecond))

		fc.shouldTxSucceed = true
		db.RemoveServerFromBlacklist(destination)
		queues.RetryServer(destination)
		checkRetry := func(log poll.LogT) poll.Result {
			pduData, dbErrPDU := db.GetPendingPDUs(pc.Context(), destination, 200)
			assert.NoError(t, dbErrPDU)
			eduData, dbErrEDU := db.GetPendingEDUs(pc.Context(), destination, 200)
			assert.NoError(t, dbErrEDU)
			if len(pduData) == 0 && len(eduData) == 0 {
				return poll.Success()
			}
			return poll.Continue("waiting for events to be removed from database. Currently present PDU: %d EDU: %d", len(pduData), len(eduData))
		}
		poll.WaitOn(t, checkRetry, poll.WithTimeout(10*time.Second), poll.WithDelay(100*time.Millisecond))
	})
}
