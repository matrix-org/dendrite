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
	"encoding/json"
	"fmt"
	"hash/fnv"
	"sync"
	"time"

	"github.com/matrix-org/dendrite/keyserver/api"
	"github.com/matrix-org/dendrite/keyserver/producers"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"
)

// DeviceListUpdater handles device list updates from remote servers.
//
// In the case where we have the prev_id for an update, the updater just stores the update (after acquiring a per-user lock).
// In the case where we do not have the prev_id for an update, the updater marks the user_id as stale and notifies
// a worker to get the latest device list for this user. Note: stream IDs are scoped per user so missing a prev_id
// for a (user, device) does not mean that DEVICE is outdated as the previous ID could be for a different device:
// we have to invalidate all devices for that user. Once the list has been fetched, the per-user lock is acquired and the
// updater stores the latest list along with the latest stream ID.
//
// On startup, the updater spins up N workers which are responsible for querying device keys from remote servers.
// Workers are scoped by homeserver domain, with one worker responsible for many domains, determined by hashing
// mod N the server name. Work is sent via a channel which just serves to "poke" the worker as the data is retrieved
// from the database (which allows us to batch requests to the same server). This has a number of desirable properties:
//   - We guarantee only 1 in-flight /keys/query request per server at any time as there is exactly 1 worker responsible
//     for that domain.
//   - We don't have unbounded growth in proportion to the number of servers (this is more important in a P2P world where
//     we have many many servers)
//   - We can adjust concurrency (at the cost of memory usage) by tuning N, to accommodate mobile devices vs servers.
// The downsides are that:
//   - Query requests can get queued behind other servers if they hash to the same worker, even if there are other free
//     workers elsewhere. Whilst suboptimal, provided we cap how long a single request can last (e.g using context timeouts)
//     we guarantee we will get around to it. Also, more users on a given server does not increase the number of requests
//     (as /keys/query allows multiple users to be specified) so being stuck behind matrix.org won't materially be any worse
//     than being stuck behind foo.bar
// In the event that the query fails, the worker spins up a short-lived goroutine whose sole purpose is to inject the server
// name back into the channel after a certain amount of time. If in the interim the device lists have been updated, then
// the database query will return no stale lists. Reinjection into the channel continues until success or the server terminates,
// when it will be reloaded on startup.
type DeviceListUpdater struct {
	// A map from user_id to a mutex. Used when we are missing prev IDs so we don't make more than 1
	// request to the remote server and race.
	// TODO: Put in an LRU cache to bound growth
	userIDToMutex map[string]*sync.Mutex
	mu            *sync.Mutex // protects UserIDToMutex

	db          DeviceListUpdaterDatabase
	producer    *producers.KeyChange
	fedClient   *gomatrixserverlib.FederationClient
	workerChans []chan gomatrixserverlib.ServerName
}

// DeviceListUpdaterDatabase is the subset of functionality from storage.Database required for the updater.
// Useful for testing.
type DeviceListUpdaterDatabase interface {
	// StaleDeviceLists returns a list of user IDs ending with the domains provided who have stale device lists.
	// If no domains are given, all user IDs with stale device lists are returned.
	StaleDeviceLists(ctx context.Context, domains []gomatrixserverlib.ServerName) ([]string, error)

	// MarkDeviceListStale sets the stale bit for this user to isStale.
	MarkDeviceListStale(ctx context.Context, userID string, isStale bool) error

	// StoreRemoteDeviceKeys persists the given keys. Keys with the same user ID and device ID will be replaced. An empty KeyJSON removes the key
	// for this (user, device). Does not modify the stream ID for keys.
	StoreRemoteDeviceKeys(ctx context.Context, keys []api.DeviceMessage) error

	// PrevIDsExists returns true if all prev IDs exist for this user.
	PrevIDsExists(ctx context.Context, userID string, prevIDs []int) (bool, error)
}

// NewDeviceListUpdater creates a new updater which fetches fresh device lists when they go stale.
func NewDeviceListUpdater(
	db DeviceListUpdaterDatabase, producer *producers.KeyChange, fedClient *gomatrixserverlib.FederationClient,
	numWorkers int,
) *DeviceListUpdater {
	return &DeviceListUpdater{
		userIDToMutex: make(map[string]*sync.Mutex),
		mu:            &sync.Mutex{},
		db:            db,
		producer:      producer,
		fedClient:     fedClient,
		workerChans:   make([]chan gomatrixserverlib.ServerName, numWorkers),
	}
}

// Start the device list updater, which will try to refresh any stale device lists.
func (u *DeviceListUpdater) Start() error {
	for i := 0; i < len(u.workerChans); i++ {
		// Allocate a small buffer per channel.
		// If the buffer limit is reached, backpressure will cause the processing of EDUs
		// to stop (in this transaction) until key requests can be made.
		ch := make(chan gomatrixserverlib.ServerName, 10)
		u.workerChans[i] = ch
		go u.worker(ch)
	}

	staleLists, err := u.db.StaleDeviceLists(context.Background(), []gomatrixserverlib.ServerName{})
	if err != nil {
		return err
	}
	for _, userID := range staleLists {
		u.notifyWorkers(userID)
	}
	return nil
}

func (u *DeviceListUpdater) mutex(userID string) *sync.Mutex {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.userIDToMutex[userID] == nil {
		u.userIDToMutex[userID] = &sync.Mutex{}
	}
	return u.userIDToMutex[userID]
}

func (u *DeviceListUpdater) Update(ctx context.Context, event gomatrixserverlib.DeviceListUpdateEvent) error {
	isDeviceListStale, err := u.update(ctx, event)
	if err != nil {
		return err
	}
	if isDeviceListStale {
		// poke workers to handle stale device lists
		u.notifyWorkers(event.UserID)
	}
	return nil
}

func (u *DeviceListUpdater) update(ctx context.Context, event gomatrixserverlib.DeviceListUpdateEvent) (bool, error) {
	mu := u.mutex(event.UserID)
	mu.Lock()
	defer mu.Unlock()
	// check if we have the prev IDs
	exists, err := u.db.PrevIDsExists(ctx, event.UserID, event.PrevID)
	if err != nil {
		return false, fmt.Errorf("failed to check prev IDs exist for %s (%s): %w", event.UserID, event.DeviceID, err)
	}
	util.GetLogger(ctx).WithFields(logrus.Fields{
		"prev_ids_exist": exists,
		"user_id":        event.UserID,
		"device_id":      event.DeviceID,
		"stream_id":      event.StreamID,
		"prev_ids":       event.PrevID,
	}).Info("DeviceListUpdater.Update")

	// if we haven't missed anything update the database and notify users
	if exists {
		keys := []api.DeviceMessage{
			{
				DeviceKeys: api.DeviceKeys{
					DeviceID:    event.DeviceID,
					DisplayName: event.DeviceDisplayName,
					KeyJSON:     event.Keys,
					UserID:      event.UserID,
				},
				StreamID: event.StreamID,
			},
		}
		err = u.db.StoreRemoteDeviceKeys(ctx, keys)
		if err != nil {
			return false, fmt.Errorf("failed to store remote device keys for %s (%s): %w", event.UserID, event.DeviceID, err)
		}
		// ALWAYS emit key changes when we've been poked over federation even if there's no change
		// just in case this poke is important for something.
		err = u.producer.ProduceKeyChanges(keys)
		if err != nil {
			return false, fmt.Errorf("failed to produce device key changes for %s (%s): %w", event.UserID, event.DeviceID, err)
		}
		return false, nil
	}

	err = u.db.MarkDeviceListStale(ctx, event.UserID, true)
	if err != nil {
		return false, fmt.Errorf("failed to mark device list for %s as stale: %w", event.UserID, err)
	}

	return true, nil
}

func (u *DeviceListUpdater) notifyWorkers(userID string) {
	_, remoteServer, err := gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		return
	}
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(remoteServer))
	index := int(hash.Sum32()) % len(u.workerChans)
	u.workerChans[index] <- remoteServer
}

func (u *DeviceListUpdater) worker(ch chan gomatrixserverlib.ServerName) {
	// It's possible to get many of the same server name in the channel, so in order
	// to prevent processing the same server over and over we keep track of when we
	// last made a request to the server. If we get the server name during the cooloff
	// period, we'll ignore the poke.
	lastProcessed := make(map[gomatrixserverlib.ServerName]time.Time)
	cooloffPeriod := time.Minute
	shouldProcess := func(srv gomatrixserverlib.ServerName) bool {
		// we should process requests when now is after the last process time + cooloff
		return time.Now().After(lastProcessed[srv].Add(cooloffPeriod))
	}

	// on failure, spin up a short-lived goroutine to inject the server name again.
	inject := func(srv gomatrixserverlib.ServerName, duration time.Duration) {
		time.Sleep(duration)
		ch <- srv
	}

	for serverName := range ch {
		if !shouldProcess(serverName) {
			// do not inject into the channel as we know there will be a sleeping goroutine
			// which will do it after the cooloff period expires
			continue
		}
		lastProcessed[serverName] = time.Now()
		shouldRetry := u.processServer(serverName)
		if shouldRetry {
			go inject(serverName, cooloffPeriod) // TODO: Backoff?
		}
	}
}

func (u *DeviceListUpdater) processServer(serverName gomatrixserverlib.ServerName) bool {
	requestTimeout := time.Minute // max amount of time we want to spend on each request
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	logger := util.GetLogger(ctx).WithField("server_name", serverName)
	// fetch stale device lists
	userIDs, err := u.db.StaleDeviceLists(ctx, []gomatrixserverlib.ServerName{serverName})
	if err != nil {
		logger.WithError(err).Error("failed to load stale device lists")
		return true
	}
	hasFailures := false
	for _, userID := range userIDs {
		if ctx.Err() != nil {
			// we've timed out, give up and go to the back of the queue to let another server be processed.
			hasFailures = true
			break
		}
		res, err := u.fedClient.GetUserDevices(ctx, serverName, userID)
		if err != nil {
			logger.WithError(err).WithField("user_id", userID).Error("failed to query device keys for user")
			hasFailures = true
			continue
		}
		err = u.updateDeviceList(ctx, &res)
		if err != nil {
			logger.WithError(err).WithField("user_id", userID).Error("fetched device list but failed to store it")
			hasFailures = true
		}
	}
	return hasFailures
}

func (u *DeviceListUpdater) updateDeviceList(ctx context.Context, res *gomatrixserverlib.RespUserDevices) error {
	keys := make([]api.DeviceMessage, len(res.Devices))
	for i, device := range res.Devices {
		keyJSON, err := json.Marshal(device.Keys)
		if err != nil {
			util.GetLogger(ctx).WithField("keys", device.Keys).Error("failed to marshal keys, skipping device")
			continue
		}
		keys[i] = api.DeviceMessage{
			StreamID: res.StreamID,
			DeviceKeys: api.DeviceKeys{
				DeviceID:    device.DeviceID,
				DisplayName: device.DisplayName,
				UserID:      res.UserID,
				KeyJSON:     keyJSON,
			},
		}
	}
	err := u.db.StoreRemoteDeviceKeys(ctx, keys)
	if err != nil {
		return err
	}
	return u.db.MarkDeviceListStale(ctx, res.UserID, false)
}
