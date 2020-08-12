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
	"io/ioutil"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/matrix-org/dendrite/keyserver/api"
	"github.com/matrix-org/gomatrixserverlib"
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
	prevIDsExist func(string, []int) bool
	storedKeys   []api.DeviceMessage
}

// StaleDeviceLists returns a list of user IDs ending with the domains provided who have stale device lists.
// If no domains are given, all user IDs with stale device lists are returned.
func (d *mockDeviceListUpdaterDatabase) StaleDeviceLists(ctx context.Context, domains []gomatrixserverlib.ServerName) ([]string, error) {
	var result []string
	for userID := range d.staleUsers {
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
	d.staleUsers[userID] = isStale
	return nil
}

// StoreRemoteDeviceKeys persists the given keys. Keys with the same user ID and device ID will be replaced. An empty KeyJSON removes the key
// for this (user, device). Does not modify the stream ID for keys.
func (d *mockDeviceListUpdaterDatabase) StoreRemoteDeviceKeys(ctx context.Context, keys []api.DeviceMessage, clear []string) error {
	d.storedKeys = append(d.storedKeys, keys...)
	return nil
}

// PrevIDsExists returns true if all prev IDs exist for this user.
func (d *mockDeviceListUpdaterDatabase) PrevIDsExists(ctx context.Context, userID string, prevIDs []int) (bool, error) {
	return d.prevIDsExist(userID, prevIDs), nil
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
		gomatrixserverlib.ServerName("example.test"), gomatrixserverlib.KeyID("ed25519:test"), pkey, true,
	)
	fedClient.Client = *gomatrixserverlib.NewClientWithTransport(true, &roundTripper{tripper})
	return fedClient
}

// Test that the device keys get persisted and emitted if we have the previous IDs.
func TestUpdateHavePrevID(t *testing.T) {
	db := &mockDeviceListUpdaterDatabase{
		staleUsers: make(map[string]bool),
		prevIDsExist: func(string, []int) bool {
			return true
		},
	}
	producer := &mockKeyChangeProducer{}
	updater := NewDeviceListUpdater(db, producer, nil, 1)
	event := gomatrixserverlib.DeviceListUpdateEvent{
		DeviceDisplayName: "Foo Bar",
		Deleted:           false,
		DeviceID:          "FOO",
		Keys:              []byte(`{"key":"value"}`),
		PrevID:            []int{0},
		StreamID:          1,
		UserID:            "@alice:localhost",
	}
	err := updater.Update(ctx, event)
	if err != nil {
		t.Fatalf("Update returned an error: %s", err)
	}
	want := api.DeviceMessage{
		StreamID: event.StreamID,
		DeviceKeys: api.DeviceKeys{
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
	if db.staleUsers[event.UserID] {
		t.Errorf("%s incorrectly marked as stale", event.UserID)
	}
}

// Test that device keys are fetched from the remote server if we are missing prev IDs
// and that the user's devices are marked as stale until it succeeds.
func TestUpdateNoPrevID(t *testing.T) {
	db := &mockDeviceListUpdaterDatabase{
		staleUsers: make(map[string]bool),
		prevIDsExist: func(string, []int) bool {
			return false
		},
	}
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
			Body: ioutil.NopCloser(strings.NewReader(`
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
	updater := NewDeviceListUpdater(db, producer, fedClient, 2)
	if err := updater.Start(); err != nil {
		t.Fatalf("failed to start updater: %s", err)
	}
	event := gomatrixserverlib.DeviceListUpdateEvent{
		DeviceDisplayName: "Mobile Phone",
		Deleted:           false,
		DeviceID:          "another_device_id",
		Keys:              []byte(`{"key":"value"}`),
		PrevID:            []int{3},
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
		StreamID: 5,
		DeviceKeys: api.DeviceKeys{
			DeviceID:    "JLAFKJWSCS",
			DisplayName: "Mobile Phone",
			UserID:      remoteUserID,
			KeyJSON:     []byte(keyJSON),
		},
	}
	// Now we should have a fresh list and the keys and emitted something
	if db.staleUsers[event.UserID] {
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
