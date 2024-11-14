// Copyright 2024 New Vector Ltd.
// Copyright 2020 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package internal

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	api2 "github.com/element-hq/dendrite/federationapi/api"
	"github.com/element-hq/dendrite/federationapi/statistics"
	"github.com/element-hq/dendrite/internal/caching"
	"github.com/element-hq/dendrite/internal/sqlutil"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/matrix-org/gomatrixserverlib/spec"

	roomserver "github.com/element-hq/dendrite/roomserver/api"
	"github.com/element-hq/dendrite/setup/config"
	"github.com/element-hq/dendrite/setup/process"
	"github.com/element-hq/dendrite/test"
	"github.com/element-hq/dendrite/userapi/api"
	"github.com/element-hq/dendrite/userapi/storage"
)

var (
	ctx = context.Background()
)

type mockKeyChangeProducer struct {
	events []api.DeviceMessage
}

func (p *mockKeyChangeProducer) ProduceKeyChanges(keys []api.DeviceMessage) error {
	p.events = append(p.events, keys...)
	return nil
}

type mockDeviceListUpdaterDatabase struct {
	staleUsers   map[string]bool
	prevIDsExist func(string, []int64) bool
	storedKeys   []api.DeviceMessage
	mu           sync.Mutex // protect staleUsers
}

func (d *mockDeviceListUpdaterDatabase) DeleteStaleDeviceLists(ctx context.Context, userIDs []string) error {
	return nil
}

// StaleDeviceLists returns a list of user IDs ending with the domains provided who have stale device lists.
// If no domains are given, all user IDs with stale device lists are returned.
func (d *mockDeviceListUpdaterDatabase) StaleDeviceLists(ctx context.Context, domains []spec.ServerName) ([]string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	var result []string
	for userID, isStale := range d.staleUsers {
		if !isStale {
			continue
		}
		_, remoteServer, err := gomatrixserverlib.SplitID('@', userID)
		if err != nil {
			return nil, err
		}
		if len(domains) == 0 {
			result = append(result, userID)
			continue
		}
		for _, d := range domains {
			if remoteServer == d {
				result = append(result, userID)
				break
			}
		}
	}
	return result, nil
}

// MarkDeviceListStale sets the stale bit for this user to isStale.
func (d *mockDeviceListUpdaterDatabase) MarkDeviceListStale(ctx context.Context, userID string, isStale bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.staleUsers[userID] = isStale
	return nil
}

func (d *mockDeviceListUpdaterDatabase) isStale(userID string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.staleUsers[userID]
}

// StoreRemoteDeviceKeys persists the given keys. Keys with the same user ID and device ID will be replaced. An empty KeyJSON removes the key
// for this (user, device). Does not modify the stream ID for keys.
func (d *mockDeviceListUpdaterDatabase) StoreRemoteDeviceKeys(ctx context.Context, keys []api.DeviceMessage, clear []string) error {
	d.storedKeys = append(d.storedKeys, keys...)
	return nil
}

// PrevIDsExists returns true if all prev IDs exist for this user.
func (d *mockDeviceListUpdaterDatabase) PrevIDsExists(ctx context.Context, userID string, prevIDs []int64) (bool, error) {
	return d.prevIDsExist(userID, prevIDs), nil
}

func (d *mockDeviceListUpdaterDatabase) DeviceKeysJSON(ctx context.Context, keys []api.DeviceMessage) error {
	return nil
}

type mockDeviceListUpdaterAPI struct {
}

func (d *mockDeviceListUpdaterAPI) PerformUploadDeviceKeys(ctx context.Context, req *api.PerformUploadDeviceKeysRequest, res *api.PerformUploadDeviceKeysResponse) {
}

var testIsBlacklistedOrBackingOff = func(s spec.ServerName) (*statistics.ServerStatistics, error) {
	return &statistics.ServerStatistics{}, nil
}

type roundTripper struct {
	fn func(*http.Request) (*http.Response, error)
}

func (t *roundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.fn(req)
}

func newFedClient(tripper func(*http.Request) (*http.Response, error)) fclient.FederationClient {
	_, pkey, _ := ed25519.GenerateKey(nil)
	fedClient := fclient.NewFederationClient(
		[]*fclient.SigningIdentity{
			{
				ServerName: spec.ServerName("example.test"),
				KeyID:      gomatrixserverlib.KeyID("ed25519:test"),
				PrivateKey: pkey,
			},
		},
		fclient.WithTransport(&roundTripper{tripper}),
	)
	return fedClient
}

// Test that the device keys get persisted and emitted if we have the previous IDs.
func TestUpdateHavePrevID(t *testing.T) {
	db := &mockDeviceListUpdaterDatabase{
		staleUsers: make(map[string]bool),
		prevIDsExist: func(string, []int64) bool {
			return true
		},
	}
	ap := &mockDeviceListUpdaterAPI{}
	producer := &mockKeyChangeProducer{}
	updater := NewDeviceListUpdater(process.NewProcessContext(), db, ap, producer, nil, 1, nil, "localhost", caching.DisableMetrics, testIsBlacklistedOrBackingOff)
	event := gomatrixserverlib.DeviceListUpdateEvent{
		DeviceDisplayName: "Foo Bar",
		Deleted:           false,
		DeviceID:          "FOO",
		Keys:              []byte(`{"key":"value"}`),
		PrevID:            []int64{0},
		StreamID:          1,
		UserID:            "@alice:localhost",
	}
	err := updater.Update(ctx, event)
	if err != nil {
		t.Fatalf("Update returned an error: %s", err)
	}
	want := api.DeviceMessage{
		Type:     api.TypeDeviceKeyUpdate,
		StreamID: event.StreamID,
		DeviceKeys: &api.DeviceKeys{
			DeviceID:    event.DeviceID,
			DisplayName: event.DeviceDisplayName,
			KeyJSON:     event.Keys,
			UserID:      event.UserID,
		},
	}
	if !reflect.DeepEqual(producer.events, []api.DeviceMessage{want}) {
		t.Errorf("Update didn't produce correct event, got %v want %v", producer.events, want)
	}
	if !reflect.DeepEqual(db.storedKeys, []api.DeviceMessage{want}) {
		t.Errorf("DB didn't store correct event, got %v want %v", db.storedKeys, want)
	}
	if db.isStale(event.UserID) {
		t.Errorf("%s incorrectly marked as stale", event.UserID)
	}
}

// Test that device keys are fetched from the remote server if we are missing prev IDs
// and that the user's devices are marked as stale until it succeeds.
func TestUpdateNoPrevID(t *testing.T) {
	db := &mockDeviceListUpdaterDatabase{
		staleUsers: make(map[string]bool),
		prevIDsExist: func(string, []int64) bool {
			return false
		},
	}
	ap := &mockDeviceListUpdaterAPI{}
	producer := &mockKeyChangeProducer{}
	remoteUserID := "@alice:example.somewhere"
	var wg sync.WaitGroup
	wg.Add(1)
	keyJSON := `{"user_id":"` + remoteUserID + `","device_id":"JLAFKJWSCS","algorithms":["m.olm.v1.curve25519-aes-sha2","m.megolm.v1.aes-sha2"],"keys":{"curve25519:JLAFKJWSCS":"3C5BFWi2Y8MaVvjM8M22DBmh24PmgR0nPvJOIArzgyI","ed25519:JLAFKJWSCS":"lEuiRJBit0IG6nUf5pUzWTUEsRVVe/HJkoKuEww9ULI"},"signatures":{"` + remoteUserID + `":{"ed25519:JLAFKJWSCS":"dSO80A01XiigH3uBiDVx/EjzaoycHcjq9lfQX0uWsqxl2giMIiSPR8a4d291W1ihKJL/a+myXS367WT6NAIcBA"}}}`
	fedClient := newFedClient(func(req *http.Request) (*http.Response, error) {
		defer wg.Done()
		if req.URL.Path != "/_matrix/federation/v1/user/devices/"+url.PathEscape(remoteUserID) {
			return nil, fmt.Errorf("test: invalid path: %s", req.URL.Path)
		}
		return &http.Response{
			StatusCode: 200,
			Body: io.NopCloser(strings.NewReader(`
			{
				"user_id": "` + remoteUserID + `",
				"stream_id": 5,
				"devices": [
				  {
					"device_id": "JLAFKJWSCS",
					"keys": ` + keyJSON + `,
					"device_display_name": "Mobile Phone"
				  }
				]
			  }
			`)),
		}, nil
	})
	updater := NewDeviceListUpdater(process.NewProcessContext(), db, ap, producer, fedClient, 2, nil, "example.test", caching.DisableMetrics, testIsBlacklistedOrBackingOff)
	if err := updater.Start(); err != nil {
		t.Fatalf("failed to start updater: %s", err)
	}
	event := gomatrixserverlib.DeviceListUpdateEvent{
		DeviceDisplayName: "Mobile Phone",
		Deleted:           false,
		DeviceID:          "another_device_id",
		Keys:              []byte(`{"key":"value"}`),
		PrevID:            []int64{3},
		StreamID:          4,
		UserID:            remoteUserID,
	}
	err := updater.Update(ctx, event)

	if err != nil {
		t.Fatalf("Update returned an error: %s", err)
	}
	t.Log("waiting for /users/devices to be called...")
	wg.Wait()
	// wait a bit for db to be updated...
	time.Sleep(100 * time.Millisecond)
	want := api.DeviceMessage{
		Type:     api.TypeDeviceKeyUpdate,
		StreamID: 5,
		DeviceKeys: &api.DeviceKeys{
			DeviceID:    "JLAFKJWSCS",
			DisplayName: "Mobile Phone",
			UserID:      remoteUserID,
			KeyJSON:     []byte(keyJSON),
		},
	}
	// Now we should have a fresh list and the keys and emitted something
	if db.isStale(event.UserID) {
		t.Errorf("%s still marked as stale", event.UserID)
	}
	if !reflect.DeepEqual(producer.events, []api.DeviceMessage{want}) {
		t.Logf("len got %d len want %d", len(producer.events[0].KeyJSON), len(want.KeyJSON))
		t.Errorf("Update didn't produce correct event, got %v want %v", producer.events, want)
	}
	if !reflect.DeepEqual(db.storedKeys, []api.DeviceMessage{want}) {
		t.Errorf("DB didn't store correct event, got %v want %v", db.storedKeys, want)
	}

}

// Test that if we make N calls to ManualUpdate for the same user, we only do it once, assuming the
// update is still ongoing.
func TestDebounce(t *testing.T) {
	t.Skipf("panic on closed channel on GHA")
	db := &mockDeviceListUpdaterDatabase{
		staleUsers: make(map[string]bool),
		prevIDsExist: func(string, []int64) bool {
			return true
		},
	}
	ap := &mockDeviceListUpdaterAPI{}
	producer := &mockKeyChangeProducer{}
	fedCh := make(chan *http.Response, 1)
	srv := spec.ServerName("example.com")
	userID := "@alice:example.com"
	keyJSON := `{"user_id":"` + userID + `","device_id":"JLAFKJWSCS","algorithms":["m.olm.v1.curve25519-aes-sha2","m.megolm.v1.aes-sha2"],"keys":{"curve25519:JLAFKJWSCS":"3C5BFWi2Y8MaVvjM8M22DBmh24PmgR0nPvJOIArzgyI","ed25519:JLAFKJWSCS":"lEuiRJBit0IG6nUf5pUzWTUEsRVVe/HJkoKuEww9ULI"},"signatures":{"` + userID + `":{"ed25519:JLAFKJWSCS":"dSO80A01XiigH3uBiDVx/EjzaoycHcjq9lfQX0uWsqxl2giMIiSPR8a4d291W1ihKJL/a+myXS367WT6NAIcBA"}}}`
	incomingFedReq := make(chan struct{})
	fedClient := newFedClient(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/_matrix/federation/v1/user/devices/"+url.PathEscape(userID) {
			return nil, fmt.Errorf("test: invalid path: %s", req.URL.Path)
		}
		close(incomingFedReq)
		return <-fedCh, nil
	})
	updater := NewDeviceListUpdater(process.NewProcessContext(), db, ap, producer, fedClient, 1, nil, "localhost", caching.DisableMetrics, testIsBlacklistedOrBackingOff)
	if err := updater.Start(); err != nil {
		t.Fatalf("failed to start updater: %s", err)
	}

	// hit this 5 times
	var wg sync.WaitGroup
	wg.Add(5)
	for i := 0; i < 5; i++ {
		go func() {
			defer wg.Done()
			if err := updater.ManualUpdate(context.Background(), srv, userID); err != nil {
				t.Errorf("ManualUpdate: %s", err)
			}
		}()
	}

	// wait until the updater hits federation
	select {
	case <-incomingFedReq:
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for updater to hit federation")
	}

	// user should be marked as stale
	if !db.isStale(userID) {
		t.Errorf("user %s not marked as stale", userID)
	}
	// now send the response over federation
	fedCh <- &http.Response{
		StatusCode: 200,
		Body: io.NopCloser(strings.NewReader(`
		{
			"user_id": "` + userID + `",
			"stream_id": 5,
			"devices": [
			  {
				"device_id": "JLAFKJWSCS",
				"keys": ` + keyJSON + `,
				"device_display_name": "Mobile Phone"
			  }
			]
		  }
		`)),
	}
	close(fedCh)
	// wait until all 5 ManualUpdates return. If we hit federation again we won't send a response
	// and should panic with read on a closed channel
	wg.Wait()

	// user is no longer stale now
	if db.isStale(userID) {
		t.Errorf("user %s is marked as stale", userID)
	}
}

func mustCreateKeyserverDB(t *testing.T, dbType test.DBType) (storage.KeyDatabase, func()) {
	t.Helper()

	connStr, clearDB := test.PrepareDBConnectionString(t, dbType)
	cm := sqlutil.NewConnectionManager(nil, config.DatabaseOptions{})
	db, err := storage.NewKeyDatabase(cm, &config.DatabaseOptions{ConnectionString: config.DataSource(connStr)})
	if err != nil {
		t.Fatal(err)
	}

	return db, clearDB
}

type mockKeyserverRoomserverAPI struct {
	leftUsers []string
}

func (m *mockKeyserverRoomserverAPI) QueryLeftUsers(ctx context.Context, req *roomserver.QueryLeftUsersRequest, res *roomserver.QueryLeftUsersResponse) error {
	res.LeftUsers = m.leftUsers
	return nil
}

func TestDeviceListUpdater_CleanUp(t *testing.T) {
	processCtx := process.NewProcessContext()

	alice := test.NewUser(t)
	bob := test.NewUser(t)

	// Bob is not joined to any of our rooms
	rsAPI := &mockKeyserverRoomserverAPI{leftUsers: []string{bob.ID}}

	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, clearDB := mustCreateKeyserverDB(t, dbType)
		defer clearDB()

		// This should not get deleted
		if err := db.MarkDeviceListStale(processCtx.Context(), alice.ID, true); err != nil {
			t.Error(err)
		}

		// this one should get deleted
		if err := db.MarkDeviceListStale(processCtx.Context(), bob.ID, true); err != nil {
			t.Error(err)
		}

		updater := NewDeviceListUpdater(processCtx, db, nil,
			nil, nil,
			0, rsAPI, "test", caching.DisableMetrics, testIsBlacklistedOrBackingOff)
		if err := updater.CleanUp(); err != nil {
			t.Error(err)
		}

		// check that we still have Alice in our stale list
		staleUsers, err := db.StaleDeviceLists(ctx, []spec.ServerName{"test"})
		if err != nil {
			t.Error(err)
		}

		// There should only be Alice
		wantCount := 1
		if count := len(staleUsers); count != wantCount {
			t.Fatalf("expected there to be %d stale device lists, got %d", wantCount, count)
		}

		if staleUsers[0] != alice.ID {
			t.Fatalf("unexpected stale device list user: %s, want %s", staleUsers[0], alice.ID)
		}
	})
}

func Test_dedupeStateList(t *testing.T) {
	alice := "@alice:localhost"
	bob := "@bob:localhost"
	charlie := "@charlie:notlocalhost"
	invalidUserID := "iaminvalid:localhost"

	tests := []struct {
		name       string
		staleLists []string
		want       []string
	}{
		{
			name:       "empty stateLists",
			staleLists: []string{},
			want:       []string{},
		},
		{
			name:       "single entry",
			staleLists: []string{alice},
			want:       []string{alice},
		},
		{
			name:       "multiple entries without dupe servers",
			staleLists: []string{alice, charlie},
			want:       []string{alice, charlie},
		},
		{
			name:       "multiple entries with dupe servers",
			staleLists: []string{alice, bob, charlie},
			want:       []string{alice, charlie},
		},
		{
			name:       "list with invalid userID",
			staleLists: []string{alice, bob, invalidUserID},
			want:       []string{alice},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dedupeStaleLists(tt.staleLists); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("dedupeStaleLists() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDeviceListUpdaterIgnoreBlacklisted(t *testing.T) {
	unreachableServer := spec.ServerName("notlocalhost")

	updater := DeviceListUpdater{
		workerChans: make([]chan spec.ServerName, 1),
		isBlacklistedOrBackingOffFn: func(s spec.ServerName) (*statistics.ServerStatistics, error) {
			switch s {
			case unreachableServer:
				return nil, &api2.FederationClientError{Blacklisted: true}
			}
			return nil, nil
		},
		mu:             &sync.Mutex{},
		userIDToChanMu: &sync.Mutex{},
		userIDToChan:   make(map[string]chan bool),
		userIDToMutex:  make(map[string]*sync.Mutex),
	}
	workerCh := make(chan spec.ServerName)
	defer close(workerCh)
	updater.workerChans[0] = workerCh

	// happy case
	alice := "@alice:localhost"
	aliceCh := updater.assignChannel(alice)
	defer updater.clearChannel(alice)

	// failing case
	bob := "@bob:" + unreachableServer
	bobCh := updater.assignChannel(string(bob))
	defer updater.clearChannel(string(bob))

	expectedServers := map[spec.ServerName]struct{}{
		"localhost": {},
	}
	unexpectedServers := make(map[spec.ServerName]struct{})

	go func() {
		for serverName := range workerCh {
			switch serverName {
			case "localhost":
				delete(expectedServers, serverName)
				aliceCh <- true // unblock notifyWorkers
			case unreachableServer: // this should not happen as it is "filtered" away by the blacklist
				unexpectedServers[serverName] = struct{}{}
				bobCh <- true
			default:
				unexpectedServers[serverName] = struct{}{}
			}
		}
	}()

	// alice is not blacklisted
	updater.notifyWorkers(alice)
	// bob is blacklisted
	updater.notifyWorkers(string(bob))

	for server := range expectedServers {
		t.Errorf("Server still in expectedServers map: %s", server)
	}

	for server := range unexpectedServers {
		t.Errorf("unexpected server in result: %s", server)
	}
}
