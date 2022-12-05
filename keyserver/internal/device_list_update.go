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
	"errors"
	"fmt"
	"hash/fnv"
	"net"
	"sync"
	"time"

	"github.com/matrix-org/gomatrix"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"

	fedsenderapi "github.com/matrix-org/dendrite/federationapi/api"
	"github.com/matrix-org/dendrite/keyserver/api"
	"github.com/matrix-org/dendrite/setup/process"
)

var (
	deviceListUpdateCount = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "dendrite",
			Subsystem: "keyserver",
			Name:      "device_list_update",
			Help:      "Number of times we have attempted to update device lists from this server",
		},
		[]string{"server"},
	)
)

const requestTimeout = time.Second * 30

func init() {
	prometheus.MustRegister(
		deviceListUpdateCount,
	)
}

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
//
// The downsides are that:
//   - Query requests can get queued behind other servers if they hash to the same worker, even if there are other free
//     workers elsewhere. Whilst suboptimal, provided we cap how long a single request can last (e.g using context timeouts)
//     we guarantee we will get around to it. Also, more users on a given server does not increase the number of requests
//     (as /keys/query allows multiple users to be specified) so being stuck behind matrix.org won't materially be any worse
//     than being stuck behind foo.bar
//
// In the event that the query fails, a lock is acquired and the server name along with the time to wait before retrying is
// set in a map. A restarter goroutine periodically probes this map and injects servers which are ready to be retried.
type DeviceListUpdater struct {
	process *process.ProcessContext
	// A map from user_id to a mutex. Used when we are missing prev IDs so we don't make more than 1
	// request to the remote server and race.
	// TODO: Put in an LRU cache to bound growth
	userIDToMutex map[string]*sync.Mutex
	mu            *sync.Mutex // protects UserIDToMutex

	db          DeviceListUpdaterDatabase
	api         DeviceListUpdaterAPI
	producer    KeyChangeProducer
	fedClient   fedsenderapi.KeyserverFederationAPI
	workerChans []chan gomatrixserverlib.ServerName
	thisServer  gomatrixserverlib.ServerName

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
	PrevIDsExists(ctx context.Context, userID string, prevIDs []int64) (bool, error)

	// DeviceKeysJSON populates the KeyJSON for the given keys. If any proided `keys` have a `KeyJSON` or `StreamID` already then it will be replaced.
	DeviceKeysJSON(ctx context.Context, keys []api.DeviceMessage) error
}

type DeviceListUpdaterAPI interface {
	PerformUploadDeviceKeys(ctx context.Context, req *api.PerformUploadDeviceKeysRequest, res *api.PerformUploadDeviceKeysResponse) error
}

// KeyChangeProducer is the interface for producers.KeyChange useful for testing.
type KeyChangeProducer interface {
	ProduceKeyChanges(keys []api.DeviceMessage) error
}

// NewDeviceListUpdater creates a new updater which fetches fresh device lists when they go stale.
func NewDeviceListUpdater(
	process *process.ProcessContext, db DeviceListUpdaterDatabase,
	api DeviceListUpdaterAPI, producer KeyChangeProducer,
	fedClient fedsenderapi.KeyserverFederationAPI, numWorkers int,
	thisServer gomatrixserverlib.ServerName,
) *DeviceListUpdater {
	return &DeviceListUpdater{
		process:        process,
		userIDToMutex:  make(map[string]*sync.Mutex),
		mu:             &sync.Mutex{},
		db:             db,
		api:            api,
		producer:       producer,
		fedClient:      fedClient,
		thisServer:     thisServer,
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
	offset, step := time.Second*10, time.Second
	if max := len(staleLists); max > 120 {
		step = (time.Second * 120) / time.Duration(max)
	}
	for _, userID := range staleLists {
		userID := userID // otherwise we are only sending the last entry
		time.AfterFunc(offset, func() {
			u.notifyWorkers(userID)
		})
		offset += step
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
	}).Trace("DeviceListUpdater.Update")

	// if we haven't missed anything update the database and notify users
	if exists || event.Deleted {
		k := event.Keys
		if event.Deleted {
			k = nil
		}
		keys := []api.DeviceMessage{
			{
				Type: api.TypeDeviceKeyUpdate,
				DeviceKeys: &api.DeviceKeys{
					DeviceID:    event.DeviceID,
					DisplayName: event.DeviceDisplayName,
					KeyJSON:     k,
					UserID:      event.UserID,
				},
				StreamID: event.StreamID,
			},
		}

		// DeviceKeysJSON will side-effect modify this, so it needs
		// to be a copy, not sharing any pointers with the above.
		deviceKeysCopy := *keys[0].DeviceKeys
		deviceKeysCopy.KeyJSON = nil
		existingKeys := []api.DeviceMessage{
			{
				Type:       keys[0].Type,
				DeviceKeys: &deviceKeysCopy,
				StreamID:   keys[0].StreamID,
			},
		}

		// fetch what keys we had already and only emit changes
		if err = u.db.DeviceKeysJSON(ctx, existingKeys); err != nil {
			// non-fatal, log and continue
			util.GetLogger(ctx).WithError(err).WithField("user_id", event.UserID).Errorf(
				"failed to query device keys json for calculating diffs",
			)
		}

		err = u.db.StoreRemoteDeviceKeys(ctx, keys, nil)
		if err != nil {
			return false, fmt.Errorf("failed to store remote device keys for %s (%s): %w", event.UserID, event.DeviceID, err)
		}

		if err = emitDeviceKeyChanges(u.producer, existingKeys, keys, false); err != nil {
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
	retries := make(map[gomatrixserverlib.ServerName]time.Time)
	retriesMu := &sync.Mutex{}
	// restarter goroutine which will inject failed servers into ch when it is time
	go func() {
		var serversToRetry []gomatrixserverlib.ServerName
		for {
			serversToRetry = serversToRetry[:0] // reuse memory
			time.Sleep(time.Second)
			retriesMu.Lock()
			now := time.Now()
			for srv, retryAt := range retries {
				if now.After(retryAt) {
					serversToRetry = append(serversToRetry, srv)
				}
			}
			for _, srv := range serversToRetry {
				delete(retries, srv)
			}
			retriesMu.Unlock()
			for _, srv := range serversToRetry {
				ch <- srv
			}
		}
	}()
	for serverName := range ch {
		retriesMu.Lock()
		_, exists := retries[serverName]
		retriesMu.Unlock()
		if exists {
			// Don't retry a server that we're already waiting for.
			continue
		}
		waitTime, shouldRetry := u.processServer(serverName)
		if shouldRetry {
			retriesMu.Lock()
			if _, exists = retries[serverName]; !exists {
				retries[serverName] = time.Now().Add(waitTime)
			}
			retriesMu.Unlock()
		}
	}
}

func (u *DeviceListUpdater) processServer(serverName gomatrixserverlib.ServerName) (time.Duration, bool) {
	ctx := u.process.Context()
	logger := util.GetLogger(ctx).WithField("server_name", serverName)
	deviceListUpdateCount.WithLabelValues(string(serverName)).Inc()

	waitTime := defaultWaitTime // How long should we wait to try again?
	successCount := 0           // How many user requests failed?

	userIDs, err := u.db.StaleDeviceLists(ctx, []gomatrixserverlib.ServerName{serverName})
	if err != nil {
		logger.WithError(err).Error("Failed to load stale device lists")
		return waitTime, true
	}

	defer func() {
		for _, userID := range userIDs {
			// always clear the channel to unblock Update calls regardless of success/failure
			u.clearChannel(userID)
		}
	}()

	for _, userID := range userIDs {
		userWait, err := u.processServerUser(ctx, serverName, userID)
		if err != nil {
			if userWait > waitTime {
				waitTime = userWait
			}
			break
		}
		successCount++
	}

	allUsersSucceeded := successCount == len(userIDs)
	if !allUsersSucceeded {
		logger.WithFields(logrus.Fields{
			"total":     len(userIDs),
			"succeeded": successCount,
			"failed":    len(userIDs) - successCount,
			"wait_time": waitTime,
		}).Debug("Failed to query device keys for some users")
	}
	return waitTime, !allUsersSucceeded
}

func (u *DeviceListUpdater) processServerUser(ctx context.Context, serverName gomatrixserverlib.ServerName, userID string) (time.Duration, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	logger := util.GetLogger(ctx).WithFields(logrus.Fields{
		"server_name": serverName,
		"user_id":     userID,
	})
	res, err := u.fedClient.GetUserDevices(ctx, u.thisServer, serverName, userID)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return time.Minute * 10, err
		}
		switch e := err.(type) {
		case *json.UnmarshalTypeError, *json.SyntaxError:
			logger.WithError(err).Debugf("Device list update for %q contained invalid JSON", userID)
			return defaultWaitTime, nil
		case *fedsenderapi.FederationClientError:
			if e.RetryAfter > 0 {
				return e.RetryAfter, err
			} else if e.Blacklisted {
				return time.Hour * 8, err
			} else if e.Code >= 300 {
				// We didn't get a real FederationClientError (e.g. in polylith mode, where gomatrix.HTTPError
				// are "converted" to FederationClientError), but we probably shouldn't hit them every $waitTime seconds.
				return hourWaitTime, err
			}
		case net.Error:
			// Use the default waitTime, if it's a timeout.
			// It probably doesn't make sense to try further users.
			if !e.Timeout() {
				logger.WithError(e).Debug("GetUserDevices returned net.Error")
				return time.Minute * 10, err
			}
		case gomatrix.HTTPError:
			// The remote server returned an error, give it some time to recover.
			// This is to avoid spamming remote servers, which may not be Matrix servers anymore.
			if e.Code >= 300 {
				logger.WithError(e).Debug("GetUserDevices returned gomatrix.HTTPError")
				return hourWaitTime, err
			}
		default:
			// Something else failed
			logger.WithError(err).Debugf("GetUserDevices returned unknown error type: %T", err)
			return time.Minute * 10, err
		}
	}
	if res.UserID != userID {
		logger.WithError(err).Debugf("User ID %q in device list update response doesn't match expected %q", res.UserID, userID)
		return defaultWaitTime, nil
	}
	if res.MasterKey != nil || res.SelfSigningKey != nil {
		uploadReq := &api.PerformUploadDeviceKeysRequest{
			UserID: userID,
		}
		uploadRes := &api.PerformUploadDeviceKeysResponse{}
		if res.MasterKey != nil {
			if err = sanityCheckKey(*res.MasterKey, userID, gomatrixserverlib.CrossSigningKeyPurposeMaster); err == nil {
				uploadReq.MasterKey = *res.MasterKey
			}
		}
		if res.SelfSigningKey != nil {
			if err = sanityCheckKey(*res.SelfSigningKey, userID, gomatrixserverlib.CrossSigningKeyPurposeSelfSigning); err == nil {
				uploadReq.SelfSigningKey = *res.SelfSigningKey
			}
		}
		_ = u.api.PerformUploadDeviceKeys(ctx, uploadReq, uploadRes)
	}
	err = u.updateDeviceList(&res)
	if err != nil {
		logger.WithError(err).Error("Fetched device list but failed to store/emit it")
		return defaultWaitTime, err
	}
	return defaultWaitTime, nil
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
			Type:     api.TypeDeviceKeyUpdate,
			StreamID: res.StreamID,
			DeviceKeys: &api.DeviceKeys{
				DeviceID:    device.DeviceID,
				DisplayName: device.DisplayName,
				UserID:      res.UserID,
				KeyJSON:     keyJSON,
			},
		}
		existingKeys[i] = api.DeviceMessage{
			Type: api.TypeDeviceKeyUpdate,
			DeviceKeys: &api.DeviceKeys{
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
	err = emitDeviceKeyChanges(u.producer, existingKeys, keys, false)
	if err != nil {
		return fmt.Errorf("failed to emit key changes for fresh device list: %w", err)
	}
	return nil
}
