// Copyright 2024 New Vector Ltd.
// Copyright 2022 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/element-hq/dendrite/internal/caching"
	"github.com/element-hq/dendrite/internal/sqlutil"
	"github.com/element-hq/dendrite/test/testrig"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"gotest.tools/v3/poll"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/stretchr/testify/assert"

	"github.com/element-hq/dendrite/federationapi/statistics"
	"github.com/element-hq/dendrite/federationapi/storage"
	"github.com/element-hq/dendrite/roomserver/types"
	"github.com/element-hq/dendrite/setup/config"
	"github.com/element-hq/dendrite/setup/process"
	"github.com/element-hq/dendrite/test"
)

func mustCreateFederationDatabase(t *testing.T, dbType test.DBType, realDatabase bool) (storage.Database, *process.ProcessContext, func()) {
	if realDatabase {
		// Real Database/s
		cfg, processCtx, close := testrig.CreateConfig(t, dbType)
		cm := sqlutil.NewConnectionManager(processCtx, cfg.Global.DatabaseOptions)
		caches := caching.NewRistrettoCache(128*1024*1024, time.Hour, caching.DisableMetrics)
		connStr, dbClose := test.PrepareDBConnectionString(t, dbType)
		db, err := storage.NewDatabase(processCtx.Context(), cm, &config.DatabaseOptions{
			ConnectionString: config.DataSource(connStr),
		}, caches, cfg.Global.IsLocalServerName)
		if err != nil {
			t.Fatalf("NewDatabase returned %s", err)
		}
		return db, processCtx, func() {
			close()
			dbClose()
		}
	} else {
		// Fake Database
		db := test.NewInMemoryFederationDatabase()
		return db, process.NewProcessContext(), func() {}
	}
}

type stubFederationClient struct {
	fclient.FederationClient
	shouldTxSucceed      bool
	shouldTxRelaySucceed bool
	txCount              atomic.Uint32
	txRelayCount         atomic.Uint32
}

func (f *stubFederationClient) SendTransaction(ctx context.Context, t gomatrixserverlib.Transaction) (res fclient.RespSend, err error) {
	var result error
	if !f.shouldTxSucceed {
		result = fmt.Errorf("transaction failed")
	}

	f.txCount.Add(1)
	return fclient.RespSend{}, result
}

func (f *stubFederationClient) P2PSendTransactionToRelay(ctx context.Context, u spec.UserID, t gomatrixserverlib.Transaction, forwardingServer spec.ServerName) (res fclient.EmptyResp, err error) {
	var result error
	if !f.shouldTxRelaySucceed {
		result = fmt.Errorf("relay transaction failed")
	}

	f.txRelayCount.Add(1)
	return fclient.EmptyResp{}, result
}

func mustCreatePDU(t *testing.T) *types.HeaderedEvent {
	t.Helper()
	content := `{"type":"m.room.message", "room_id":"!room:a"}`
	ev, err := gomatrixserverlib.MustGetRoomVersion(gomatrixserverlib.RoomVersionV10).NewEventFromTrustedJSON([]byte(content), false)
	if err != nil {
		t.Fatalf("failed to create event: %v", err)
	}
	return &types.HeaderedEvent{PDU: ev}
}

func mustCreateEDU(t *testing.T) *gomatrixserverlib.EDU {
	t.Helper()
	return &gomatrixserverlib.EDU{Type: spec.MTyping}
}

func testSetup(failuresUntilBlacklist uint32, failuresUntilAssumedOffline uint32, shouldTxSucceed bool, shouldTxRelaySucceed bool, t *testing.T, dbType test.DBType, realDatabase bool) (storage.Database, *stubFederationClient, *OutgoingQueues, *process.ProcessContext, func()) {
	db, processContext, close := mustCreateFederationDatabase(t, dbType, realDatabase)

	fc := &stubFederationClient{
		shouldTxSucceed:      shouldTxSucceed,
		shouldTxRelaySucceed: shouldTxRelaySucceed,
		txCount:              atomic.Uint32{},
		txRelayCount:         atomic.Uint32{},
	}

	stats := statistics.NewStatistics(db, failuresUntilBlacklist, failuresUntilAssumedOffline, false)
	signingInfo := []*fclient.SigningIdentity{
		{
			KeyID:      "ed21019:auto",
			PrivateKey: test.PrivateKeyA,
			ServerName: "localhost",
		},
	}
	queues := NewOutgoingQueues(db, processContext, false, "localhost", fc, &stats, signingInfo)

	return db, fc, queues, processContext, close
}

func TestSendPDUOnSuccessRemovedFromDB(t *testing.T) {
	t.Parallel()
	failuresUntilBlacklist := uint32(16)
	destination := spec.ServerName("remotehost")
	db, fc, queues, pc, close := testSetup(failuresUntilBlacklist, failuresUntilBlacklist+1, true, false, t, test.DBTypeSQLite, false)
	defer close()
	defer func() {
		pc.ShutdownDendrite()
		<-pc.WaitForShutdown()
	}()

	ev := mustCreatePDU(t)
	err := queues.SendEvent(ev, "localhost", []spec.ServerName{destination})
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
	destination := spec.ServerName("remotehost")
	db, fc, queues, pc, close := testSetup(failuresUntilBlacklist, failuresUntilBlacklist+1, true, false, t, test.DBTypeSQLite, false)
	defer close()
	defer func() {
		pc.ShutdownDendrite()
		<-pc.WaitForShutdown()
	}()

	ev := mustCreateEDU(t)
	err := queues.SendEDU(ev, "localhost", []spec.ServerName{destination})
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
	destination := spec.ServerName("remotehost")
	db, fc, queues, pc, close := testSetup(failuresUntilBlacklist, failuresUntilBlacklist+1, false, false, t, test.DBTypeSQLite, false)
	defer close()
	defer func() {
		pc.ShutdownDendrite()
		<-pc.WaitForShutdown()
	}()

	ev := mustCreatePDU(t)
	err := queues.SendEvent(ev, "localhost", []spec.ServerName{destination})
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
	destination := spec.ServerName("remotehost")
	db, fc, queues, pc, close := testSetup(failuresUntilBlacklist, failuresUntilBlacklist+1, false, false, t, test.DBTypeSQLite, false)
	defer close()
	defer func() {
		pc.ShutdownDendrite()
		<-pc.WaitForShutdown()
	}()

	ev := mustCreateEDU(t)
	err := queues.SendEDU(ev, "localhost", []spec.ServerName{destination})
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
	destination := spec.ServerName("remotehost")
	db, fc, queues, pc, close := testSetup(failuresUntilBlacklist, failuresUntilBlacklist+1, false, false, t, test.DBTypeSQLite, false)
	defer close()
	defer func() {
		pc.ShutdownDendrite()
		<-pc.WaitForShutdown()
	}()

	ev := mustCreatePDU(t)
	err := queues.SendEvent(ev, "localhost", []spec.ServerName{destination})
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
	err = queues.SendEvent(ev, "localhost", []spec.ServerName{destination})
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
	destination := spec.ServerName("remotehost")
	db, fc, queues, pc, close := testSetup(failuresUntilBlacklist, failuresUntilBlacklist+1, false, false, t, test.DBTypeSQLite, false)
	defer close()
	defer func() {
		pc.ShutdownDendrite()
		<-pc.WaitForShutdown()
	}()

	ev := mustCreateEDU(t)
	err := queues.SendEDU(ev, "localhost", []spec.ServerName{destination})
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
	err = queues.SendEDU(ev, "localhost", []spec.ServerName{destination})
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
	destination := spec.ServerName("remotehost")
	db, fc, queues, pc, close := testSetup(failuresUntilBlacklist, failuresUntilBlacklist+1, false, false, t, test.DBTypeSQLite, false)
	defer close()
	defer func() {
		pc.ShutdownDendrite()
		<-pc.WaitForShutdown()
	}()

	ev := mustCreatePDU(t)
	err := queues.SendEvent(ev, "localhost", []spec.ServerName{destination})
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
	destination := spec.ServerName("remotehost")
	db, fc, queues, pc, close := testSetup(failuresUntilBlacklist, failuresUntilBlacklist+1, false, false, t, test.DBTypeSQLite, false)
	defer close()
	defer func() {
		pc.ShutdownDendrite()
		<-pc.WaitForShutdown()
	}()

	ev := mustCreateEDU(t)
	err := queues.SendEDU(ev, "localhost", []spec.ServerName{destination})
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
	destination := spec.ServerName("remotehost")
	db, fc, queues, pc, close := testSetup(failuresUntilBlacklist, failuresUntilBlacklist+1, false, false, t, test.DBTypeSQLite, false)
	defer close()
	defer func() {
		pc.ShutdownDendrite()
		<-pc.WaitForShutdown()
	}()

	queues.statistics.ForServer(destination).Failure()

	ev := mustCreatePDU(t)
	err := queues.SendEvent(ev, "localhost", []spec.ServerName{destination})
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
	destination := spec.ServerName("remotehost")
	db, fc, queues, pc, close := testSetup(failuresUntilBlacklist, failuresUntilBlacklist+1, false, false, t, test.DBTypeSQLite, false)
	defer close()
	defer func() {
		pc.ShutdownDendrite()
		<-pc.WaitForShutdown()
	}()

	queues.statistics.ForServer(destination).Failure()

	ev := mustCreateEDU(t)
	err := queues.SendEDU(ev, "localhost", []spec.ServerName{destination})
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
	destination := spec.ServerName("remotehost")
	db, fc, queues, pc, close := testSetup(failuresUntilBlacklist, failuresUntilBlacklist+1, false, false, t, test.DBTypeSQLite, false)
	defer close()
	defer func() {
		pc.ShutdownDendrite()
		<-pc.WaitForShutdown()
	}()

	// NOTE : getQueue before sending event to ensure we grab the same queue reference
	// before it is blacklisted and deleted.
	dest := queues.getQueue(destination)
	ev := mustCreatePDU(t)
	err := queues.SendEvent(ev, "localhost", []spec.ServerName{destination})
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
	wasBlacklisted := dest.statistics.MarkServerAlive()
	queues.RetryServer(destination, wasBlacklisted)
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
	destination := spec.ServerName("remotehost")
	db, fc, queues, pc, close := testSetup(failuresUntilBlacklist, failuresUntilBlacklist+1, false, false, t, test.DBTypeSQLite, false)
	defer close()
	defer func() {
		pc.ShutdownDendrite()
		<-pc.WaitForShutdown()
	}()

	// NOTE : getQueue before sending event to ensure we grab the same queue reference
	// before it is blacklisted and deleted.
	dest := queues.getQueue(destination)
	ev := mustCreateEDU(t)
	err := queues.SendEDU(ev, "localhost", []spec.ServerName{destination})
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
	wasBlacklisted := dest.statistics.MarkServerAlive()
	queues.RetryServer(destination, wasBlacklisted)
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
	destination := spec.ServerName("remotehost")

	// test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
	// db, fc, queues, pc, close := testSetup(failuresUntilBlacklist, true, t, dbType, true)
	db, fc, queues, pc, close := testSetup(failuresUntilBlacklist, failuresUntilBlacklist+1, true, false, t, test.DBTypeSQLite, false)
	defer close()
	defer func() {
		pc.ShutdownDendrite()
		<-pc.WaitForShutdown()
	}()

	destinations := map[spec.ServerName]struct{}{destination: {}}
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
	err := queues.SendEvent(ev, "localhost", []spec.ServerName{destination})
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
	destination := spec.ServerName("remotehost")

	// test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
	// db, fc, queues, pc, close := testSetup(failuresUntilBlacklist, true, t, dbType, true)
	db, fc, queues, pc, close := testSetup(failuresUntilBlacklist, failuresUntilBlacklist+1, true, false, t, test.DBTypeSQLite, false)
	defer close()
	defer func() {
		pc.ShutdownDendrite()
		<-pc.WaitForShutdown()
	}()

	destinations := map[spec.ServerName]struct{}{destination: {}}
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
	err := queues.SendEDU(ev, "localhost", []spec.ServerName{destination})
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
	destination := spec.ServerName("remotehost")

	// test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
	// db, fc, queues, pc, close := testSetup(failuresUntilBlacklist, true, t, dbType, true)
	db, fc, queues, pc, close := testSetup(failuresUntilBlacklist, failuresUntilBlacklist+1, true, false, t, test.DBTypeSQLite, false)
	defer close()
	defer func() {
		pc.ShutdownDendrite()
		<-pc.WaitForShutdown()
	}()

	destinations := map[spec.ServerName]struct{}{destination: {}}
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
	err := queues.SendEDU(ev, "localhost", []spec.ServerName{destination})
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
	destination := spec.ServerName("remotehost")
	db, fc, queues, pc, close := testSetup(failuresUntilBlacklist, failuresUntilBlacklist+1, true, false, t, test.DBTypeSQLite, false)
	defer close()
	defer func() {
		pc.ShutdownDendrite()
		<-pc.WaitForShutdown()
	}()

	dest := queues.getQueue(destination)
	queues.statistics.ForServer(destination).Failure()
	destinations := map[spec.ServerName]struct{}{destination: {}}
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
	destination := spec.ServerName("remotehost")
	destinations := map[spec.ServerName]struct{}{destination: {}}
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, fc, queues, pc, close := testSetup(failuresUntilBlacklist, failuresUntilBlacklist+1, false, false, t, dbType, true)
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
		err := queues.SendEvent(ev, "localhost", []spec.ServerName{destination})
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
		wasBlacklisted := dest.statistics.MarkServerAlive()
		queues.RetryServer(destination, wasBlacklisted)
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

func TestSendPDUMultipleFailuresAssumedOffline(t *testing.T) {
	t.Parallel()
	failuresUntilBlacklist := uint32(7)
	failuresUntilAssumedOffline := uint32(2)
	destination := spec.ServerName("remotehost")
	db, fc, queues, pc, close := testSetup(failuresUntilBlacklist, failuresUntilAssumedOffline, false, false, t, test.DBTypeSQLite, false)
	defer close()
	defer func() {
		pc.ShutdownDendrite()
		<-pc.WaitForShutdown()
	}()

	ev := mustCreatePDU(t)
	err := queues.SendEvent(ev, "localhost", []spec.ServerName{destination})
	assert.NoError(t, err)

	check := func(log poll.LogT) poll.Result {
		if fc.txCount.Load() == failuresUntilAssumedOffline {
			data, dbErr := db.GetPendingPDUs(pc.Context(), destination, 100)
			assert.NoError(t, dbErr)
			if len(data) == 1 {
				if val, _ := db.IsServerAssumedOffline(context.Background(), destination); val {
					return poll.Success()
				}
				return poll.Continue("waiting for server to be assumed offline")
			}
			return poll.Continue("waiting for event to be added to database. Currently present PDU: %d", len(data))
		}
		return poll.Continue("waiting for more send attempts before checking database. Currently %d", fc.txCount.Load())
	}
	poll.WaitOn(t, check, poll.WithTimeout(5*time.Second), poll.WithDelay(100*time.Millisecond))
}

func TestSendEDUMultipleFailuresAssumedOffline(t *testing.T) {
	t.Parallel()
	failuresUntilBlacklist := uint32(7)
	failuresUntilAssumedOffline := uint32(2)
	destination := spec.ServerName("remotehost")
	db, fc, queues, pc, close := testSetup(failuresUntilBlacklist, failuresUntilAssumedOffline, false, false, t, test.DBTypeSQLite, false)
	defer close()
	defer func() {
		pc.ShutdownDendrite()
		<-pc.WaitForShutdown()
	}()

	ev := mustCreateEDU(t)
	err := queues.SendEDU(ev, "localhost", []spec.ServerName{destination})
	assert.NoError(t, err)

	check := func(log poll.LogT) poll.Result {
		if fc.txCount.Load() == failuresUntilAssumedOffline {
			data, dbErr := db.GetPendingEDUs(pc.Context(), destination, 100)
			assert.NoError(t, dbErr)
			if len(data) == 1 {
				if val, _ := db.IsServerAssumedOffline(context.Background(), destination); val {
					return poll.Success()
				}
				return poll.Continue("waiting for server to be assumed offline")
			}
			return poll.Continue("waiting for event to be added to database. Currently present EDU: %d", len(data))
		}
		return poll.Continue("waiting for more send attempts before checking database. Currently %d", fc.txCount.Load())
	}
	poll.WaitOn(t, check, poll.WithTimeout(5*time.Second), poll.WithDelay(100*time.Millisecond))
}

func TestSendPDUOnRelaySuccessRemovedFromDB(t *testing.T) {
	t.Parallel()
	failuresUntilBlacklist := uint32(16)
	failuresUntilAssumedOffline := uint32(1)
	destination := spec.ServerName("remotehost")
	db, fc, queues, pc, close := testSetup(failuresUntilBlacklist, failuresUntilAssumedOffline, false, true, t, test.DBTypeSQLite, false)
	defer close()
	defer func() {
		pc.ShutdownDendrite()
		<-pc.WaitForShutdown()
	}()

	relayServers := []spec.ServerName{"relayserver"}
	queues.statistics.ForServer(destination).AddRelayServers(relayServers)

	ev := mustCreatePDU(t)
	err := queues.SendEvent(ev, "localhost", []spec.ServerName{destination})
	assert.NoError(t, err)

	check := func(log poll.LogT) poll.Result {
		if fc.txCount.Load() >= 1 {
			if fc.txRelayCount.Load() == 1 {
				data, dbErr := db.GetPendingPDUs(pc.Context(), destination, 100)
				assert.NoError(t, dbErr)
				if len(data) == 0 {
					return poll.Success()
				}
				return poll.Continue("waiting for event to be removed from database. Currently present PDU: %d", len(data))
			}
			return poll.Continue("waiting for more relay send attempts before checking database. Currently %d", fc.txRelayCount.Load())
		}
		return poll.Continue("waiting for more send attempts before checking database. Currently %d", fc.txCount.Load())
	}
	poll.WaitOn(t, check, poll.WithTimeout(5*time.Second), poll.WithDelay(100*time.Millisecond))

	assumedOffline, _ := db.IsServerAssumedOffline(context.Background(), destination)
	assert.Equal(t, true, assumedOffline)
}

func TestSendEDUOnRelaySuccessRemovedFromDB(t *testing.T) {
	t.Parallel()
	failuresUntilBlacklist := uint32(16)
	failuresUntilAssumedOffline := uint32(1)
	destination := spec.ServerName("remotehost")
	db, fc, queues, pc, close := testSetup(failuresUntilBlacklist, failuresUntilAssumedOffline, false, true, t, test.DBTypeSQLite, false)
	defer close()
	defer func() {
		pc.ShutdownDendrite()
		<-pc.WaitForShutdown()
	}()

	relayServers := []spec.ServerName{"relayserver"}
	queues.statistics.ForServer(destination).AddRelayServers(relayServers)

	ev := mustCreateEDU(t)
	err := queues.SendEDU(ev, "localhost", []spec.ServerName{destination})
	assert.NoError(t, err)

	check := func(log poll.LogT) poll.Result {
		if fc.txCount.Load() >= 1 {
			if fc.txRelayCount.Load() == 1 {
				data, dbErr := db.GetPendingEDUs(pc.Context(), destination, 100)
				assert.NoError(t, dbErr)
				if len(data) == 0 {
					return poll.Success()
				}
				return poll.Continue("waiting for event to be removed from database. Currently present EDU: %d", len(data))
			}
			return poll.Continue("waiting for more relay send attempts before checking database. Currently %d", fc.txRelayCount.Load())
		}
		return poll.Continue("waiting for more send attempts before checking database. Currently %d", fc.txCount.Load())
	}
	poll.WaitOn(t, check, poll.WithTimeout(5*time.Second), poll.WithDelay(100*time.Millisecond))

	assumedOffline, _ := db.IsServerAssumedOffline(context.Background(), destination)
	assert.Equal(t, true, assumedOffline)
}
