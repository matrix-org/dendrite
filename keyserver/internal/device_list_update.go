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

	fedsenderapi "github.com/matrix-org/dendrite/federationsender/api"
	"github.com/matrix-org/dendrite/keyserver/api"
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
	producer    KeyChangeProducer
	fedClient   fedsenderapi.FederationClient
	workerChans []chan gomatrixserverlib.ServerName

	// When device lists are stale for a user, they get inserted into this map with a channel which `Update` will
	// block on or timeout via a select.
	userIDToChan   map[string]chan bool
	userIDToChanMu *sync.Mutex
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
	// for this (user, device). Does not modify the stream ID for keys. User IDs in `clearUserIDs` will have all their device keys deleted prior
	// to insertion - use this when you have a complete snapshot of a user's keys in order to track device deletions correctly.
	StoreRemoteDeviceKeys(ctx context.Context, keys []api.DeviceMessage, clearUserIDs []string) error

	// PrevIDsExists returns true if all prev IDs exist for this user.
	PrevIDsExists(ctx context.Context, userID string, prevIDs []int) (bool, error)

	// DeviceKeysJSON populates the KeyJSON for the given keys. If any proided `keys` have a `KeyJSON` or `StreamID` already then it will be replaced.
	DeviceKeysJSON(ctx context.Context, keys []api.DeviceMessage) error
}

// KeyChangeProducer is the interface for producers.KeyChange useful for testing.
type KeyChangeProducer interface {
	ProduceKeyChanges(keys []api.DeviceMessage) error
}

// NewDeviceListUpdater creates a new updater which fetches fresh device lists when they go stale.
func NewDeviceListUpdater(
	db DeviceListUpdaterDatabase, producer KeyChangeProducer, fedClient fedsenderapi.FederationClient,
	numWorkers int,
) *DeviceListUpdater {
	return &DeviceListUpdater{
		userIDToMutex:  make(map[string]*sync.Mutex),
		mu:             &sync.Mutex{},
		db:             db,
		producer:       producer,
		fedClient:      fedClient,
		workerChans:    make([]chan gomatrixserverlib.ServerName, numWorkers),
		userIDToChan:   make(map[string]chan bool),
		userIDToChanMu: &sync.Mutex{},
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

// ManualUpdate invalidates the device list for the given user and fetches the latest and tracks it.
// Blocks until the device list is synced or the timeout is reached.
func (u *DeviceListUpdater) ManualUpdate(ctx context.Context, serverName gomatrixserverlib.ServerName, userID string) error {
	mu := u.mutex(userID)
	mu.Lock()
	err := u.db.MarkDeviceListStale(ctx, userID, true)
	mu.Unlock()
	if err != nil {
		return fmt.Errorf("ManualUpdate: failed to mark device list for %s as stale: %w", userID, err)
	}
	u.notifyWorkers(userID)
	return nil
}

// Update blocks until the update has been stored in the database. It blocks primarily for satisfying sytest,
// which assumes when /send 200 OKs that the device lists have been updated.
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
	// if this is the first time we're hearing about this user, sync the device list manually.
	if len(event.PrevID) == 0 {
		exists = false
	}
	util.GetLogger(ctx).WithFields(logrus.Fields{
		"prev_ids_exist": exists,
		"user_id":        event.UserID,
		"device_id":      event.DeviceID,
		"stream_id":      event.StreamID,
		"prev_ids":       event.PrevID,
		"display_name":   event.DeviceDisplayName,
		"deleted":        event.Deleted,
	}).Info("DeviceListUpdater.Update")

	// if we haven't missed anything update the database and notify users
	if exists {
		k := event.Keys
		if event.Deleted {
			k = nil
		}
		keys := []api.DeviceMessage{
			{
				DeviceKeys: api.DeviceKeys{
					DeviceID:    event.DeviceID,
					DisplayName: event.DeviceDisplayName,
					KeyJSON:     k,
					UserID:      event.UserID,
				},
				StreamID: event.StreamID,
			},
		}
		err = u.db.StoreRemoteDeviceKeys(ctx, keys, nil)
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
	index := int(int64(hash.Sum32()) % int64(len(u.workerChans)))

	ch := u.assignChannel(userID)
	u.workerChans[index] <- remoteServer
	select {
	case <-ch:
	case <-time.After(10 * time.Second):
		// we don't return an error in this case as it's not a failure condition.
		// we mainly block for the benefit of sytest anyway
	}
}

func (u *DeviceListUpdater) assignChannel(userID string) chan bool {
	u.userIDToChanMu.Lock()
	defer u.userIDToChanMu.Unlock()
	if ch, ok := u.userIDToChan[userID]; ok {
		return ch
	}
	ch := make(chan bool)
	u.userIDToChan[userID] = ch
	return ch
}

func (u *DeviceListUpdater) clearChannel(userID string) {
	u.userIDToChanMu.Lock()
	defer u.userIDToChanMu.Unlock()
	if ch, ok := u.userIDToChan[userID]; ok {
		close(ch)
		delete(u.userIDToChan, userID)
	}
}

func (u *DeviceListUpdater) worker(ch chan gomatrixserverlib.ServerName) {
	// It's possible to get many of the same server name in the channel, so in order
	// to prevent processing the same server over and over we keep track of when we
	// last made a request to the server. If we get the server name during the cooloff
	// period, we'll ignore the poke.
	lastProcessed := make(map[gomatrixserverlib.ServerName]time.Time)
	// this can't be too long else sytest will give up trying to do a test
	cooloffPeriod := 500 * time.Millisecond
	shouldProcess := func(srv gomatrixserverlib.ServerName) bool {
		// we should process requests when now is after the last process time + cooloff
		return time.Now().After(lastProcessed[srv].Add(cooloffPeriod))
	}

	// on failure, spin up a short-lived goroutine to inject the server name again.
	scheduledRetries := make(map[gomatrixserverlib.ServerName]time.Time)
	inject := func(srv gomatrixserverlib.ServerName, duration time.Duration) {
		time.Sleep(duration)
		ch <- srv
	}

	for serverName := range ch {
		if !shouldProcess(serverName) {
			if time.Now().Before(scheduledRetries[serverName]) {
				// do not inject into the channel as we know there will be a sleeping goroutine
				// which will do it after the cooloff period expires
				continue
			} else {
				scheduledRetries[serverName] = time.Now().Add(cooloffPeriod)
				go inject(serverName, cooloffPeriod)
				continue
			}
		}
		lastProcessed[serverName] = time.Now()
		waitTime, shouldRetry := u.processServer(serverName)
		if shouldRetry {
			scheduledRetries[serverName] = time.Now().Add(waitTime)
			go inject(serverName, waitTime)
		}
	}
}

func (u *DeviceListUpdater) processServer(serverName gomatrixserverlib.ServerName) (time.Duration, bool) {
	requestTimeout := time.Second * 30 // max amount of time we want to spend on each request
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	logger := util.GetLogger(ctx).WithField("server_name", serverName)
	waitTime := 2 * time.Second
	// fetch stale device lists
	userIDs, err := u.db.StaleDeviceLists(ctx, []gomatrixserverlib.ServerName{serverName})
	if err != nil {
		logger.WithError(err).Error("failed to load stale device lists")
		return waitTime, true
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
			fcerr, ok := err.(*fedsenderapi.FederationClientError)
			if ok {
				if fcerr.RetryAfter > 0 {
					waitTime = fcerr.RetryAfter
				} else if fcerr.Blacklisted {
					waitTime = time.Hour * 8
				}
			}
			hasFailures = true
			continue
		}
		err = u.updateDeviceList(&res)
		if err != nil {
			logger.WithError(err).WithField("user_id", userID).Error("fetched device list but failed to store/emit it")
			hasFailures = true
		}
	}
	for _, userID := range userIDs {
		// always clear the channel to unblock Update calls regardless of success/failure
		u.clearChannel(userID)
	}
	return waitTime, hasFailures
}

func (u *DeviceListUpdater) updateDeviceList(res *gomatrixserverlib.RespUserDevices) error {
	ctx := context.Background() // we've got the keys, don't time out when persisting them to the database.
	keys := make([]api.DeviceMessage, len(res.Devices))
	existingKeys := make([]api.DeviceMessage, len(res.Devices))
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
		existingKeys[i] = api.DeviceMessage{
			DeviceKeys: api.DeviceKeys{
				UserID:   res.UserID,
				DeviceID: device.DeviceID,
			},
		}
	}
	// fetch what keys we had already and only emit changes
	if err := u.db.DeviceKeysJSON(ctx, existingKeys); err != nil {
		// non-fatal, log and continue
		util.GetLogger(ctx).WithError(err).WithField("user_id", res.UserID).Errorf(
			"failed to query device keys json for calculating diffs",
		)
	}

	err := u.db.StoreRemoteDeviceKeys(ctx, keys, []string{res.UserID})
	if err != nil {
		return fmt.Errorf("failed to store remote device keys: %w", err)
	}
	err = u.db.MarkDeviceListStale(ctx, res.UserID, false)
	if err != nil {
		return fmt.Errorf("failed to mark device list as fresh: %w", err)
	}
	err = emitDeviceKeyChanges(u.producer, existingKeys, keys)
	if err != nil {
		return fmt.Errorf("failed to emit key changes for fresh device list: %w", err)
	}
	return nil
}
