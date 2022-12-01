// Copyright 2020 The Matrix.org Foundation C.I.C.
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
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/matrix-org/gomatrixserverlib"

	"github.com/matrix-org/dendrite/keyserver/api"
	"github.com/matrix-org/dendrite/setup/process"
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

// StaleDeviceLists returns a list of user IDs ending with the domains provided who have stale device lists.
// If no domains are given, all user IDs with stale device lists are returned.
func (d *mockDeviceListUpdaterDatabase) StaleDeviceLists(ctx context.Context, domains []gomatrixserverlib.ServerName) ([]string, error) {
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

func (d *mockDeviceListUpdaterAPI) PerformUploadDeviceKeys(ctx context.Context, req *api.PerformUploadDeviceKeysRequest, res *api.PerformUploadDeviceKeysResponse) error {
	return nil
}

type roundTripper struct {
	fn func(*http.Request) (*http.Response, error)
}

func (t *roundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.fn(req)
}

func newFedClient(tripper func(*http.Request) (*http.Response, error)) *gomatrixserverlib.FederationClient {
	_, pkey, _ := ed25519.GenerateKey(nil)
	fedClient := gomatrixserverlib.NewFederationClient(
		[]*gomatrixserverlib.SigningIdentity{
			{
				ServerName: gomatrixserverlib.ServerName("example.test"),
				KeyID:      gomatrixserverlib.KeyID("ed25519:test"),
				PrivateKey: pkey,
			},
		},
	)
	fedClient.Client = *gomatrixserverlib.NewClient(
		gomatrixserverlib.WithTransport(&roundTripper{tripper}),
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
	updater := NewDeviceListUpdater(process.NewProcessContext(), db, ap, producer, nil, 1, "localhost")
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
	updater := NewDeviceListUpdater(process.NewProcessContext(), db, ap, producer, fedClient, 2, "example.test")
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
	srv := gomatrixserverlib.ServerName("example.com")
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
	updater := NewDeviceListUpdater(process.NewProcessContext(), db, ap, producer, fedClient, 1, "localhost")
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
