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

package test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/matrix-org/dendrite/federationapi/storage/shared/receipt"
	"github.com/matrix-org/dendrite/federationapi/types"
	"github.com/matrix-org/gomatrixserverlib"
)

var nidMutex sync.Mutex
var nid = int64(0)

type InMemoryFederationDatabase struct {
	dbMutex            sync.Mutex
	pendingPDUServers  map[gomatrixserverlib.ServerName]struct{}
	pendingEDUServers  map[gomatrixserverlib.ServerName]struct{}
	blacklistedServers map[gomatrixserverlib.ServerName]struct{}
	assumedOffline     map[gomatrixserverlib.ServerName]struct{}
	pendingPDUs        map[*receipt.Receipt]*gomatrixserverlib.HeaderedEvent
	pendingEDUs        map[*receipt.Receipt]*gomatrixserverlib.EDU
	associatedPDUs     map[gomatrixserverlib.ServerName]map[*receipt.Receipt]struct{}
	associatedEDUs     map[gomatrixserverlib.ServerName]map[*receipt.Receipt]struct{}
	relayServers       map[gomatrixserverlib.ServerName][]gomatrixserverlib.ServerName
}

func NewInMemoryFederationDatabase() *InMemoryFederationDatabase {
	return &InMemoryFederationDatabase{
		pendingPDUServers:  make(map[gomatrixserverlib.ServerName]struct{}),
		pendingEDUServers:  make(map[gomatrixserverlib.ServerName]struct{}),
		blacklistedServers: make(map[gomatrixserverlib.ServerName]struct{}),
		assumedOffline:     make(map[gomatrixserverlib.ServerName]struct{}),
		pendingPDUs:        make(map[*receipt.Receipt]*gomatrixserverlib.HeaderedEvent),
		pendingEDUs:        make(map[*receipt.Receipt]*gomatrixserverlib.EDU),
		associatedPDUs:     make(map[gomatrixserverlib.ServerName]map[*receipt.Receipt]struct{}),
		associatedEDUs:     make(map[gomatrixserverlib.ServerName]map[*receipt.Receipt]struct{}),
		relayServers:       make(map[gomatrixserverlib.ServerName][]gomatrixserverlib.ServerName),
	}
}

func (d *InMemoryFederationDatabase) StoreJSON(
	ctx context.Context,
	js string,
) (*receipt.Receipt, error) {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	var event gomatrixserverlib.HeaderedEvent
	if err := json.Unmarshal([]byte(js), &event); err == nil {
		nidMutex.Lock()
		defer nidMutex.Unlock()
		nid++
		newReceipt := receipt.NewReceipt(nid)
		d.pendingPDUs[&newReceipt] = &event
		return &newReceipt, nil
	}

	var edu gomatrixserverlib.EDU
	if err := json.Unmarshal([]byte(js), &edu); err == nil {
		nidMutex.Lock()
		defer nidMutex.Unlock()
		nid++
		newReceipt := receipt.NewReceipt(nid)
		d.pendingEDUs[&newReceipt] = &edu
		return &newReceipt, nil
	}

	return nil, errors.New("Failed to determine type of json to store")
}

func (d *InMemoryFederationDatabase) GetPendingPDUs(
	ctx context.Context,
	serverName gomatrixserverlib.ServerName,
	limit int,
) (pdus map[*receipt.Receipt]*gomatrixserverlib.HeaderedEvent, err error) {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	pduCount := 0
	pdus = make(map[*receipt.Receipt]*gomatrixserverlib.HeaderedEvent)
	if receipts, ok := d.associatedPDUs[serverName]; ok {
		for dbReceipt := range receipts {
			if event, ok := d.pendingPDUs[dbReceipt]; ok {
				pdus[dbReceipt] = event
				pduCount++
				if pduCount == limit {
					break
				}
			}
		}
	}
	return pdus, nil
}

func (d *InMemoryFederationDatabase) GetPendingEDUs(
	ctx context.Context,
	serverName gomatrixserverlib.ServerName,
	limit int,
) (edus map[*receipt.Receipt]*gomatrixserverlib.EDU, err error) {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	eduCount := 0
	edus = make(map[*receipt.Receipt]*gomatrixserverlib.EDU)
	if receipts, ok := d.associatedEDUs[serverName]; ok {
		for dbReceipt := range receipts {
			if event, ok := d.pendingEDUs[dbReceipt]; ok {
				edus[dbReceipt] = event
				eduCount++
				if eduCount == limit {
					break
				}
			}
		}
	}
	return edus, nil
}

func (d *InMemoryFederationDatabase) AssociatePDUWithDestinations(
	ctx context.Context,
	destinations map[gomatrixserverlib.ServerName]struct{},
	dbReceipt *receipt.Receipt,
) error {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	if _, ok := d.pendingPDUs[dbReceipt]; ok {
		for destination := range destinations {
			if _, ok := d.associatedPDUs[destination]; !ok {
				d.associatedPDUs[destination] = make(map[*receipt.Receipt]struct{})
			}
			d.associatedPDUs[destination][dbReceipt] = struct{}{}
		}

		return nil
	} else {
		return errors.New("PDU doesn't exist")
	}
}

func (d *InMemoryFederationDatabase) AssociateEDUWithDestinations(
	ctx context.Context,
	destinations map[gomatrixserverlib.ServerName]struct{},
	dbReceipt *receipt.Receipt,
	eduType string,
	expireEDUTypes map[string]time.Duration,
) error {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	if _, ok := d.pendingEDUs[dbReceipt]; ok {
		for destination := range destinations {
			if _, ok := d.associatedEDUs[destination]; !ok {
				d.associatedEDUs[destination] = make(map[*receipt.Receipt]struct{})
			}
			d.associatedEDUs[destination][dbReceipt] = struct{}{}
		}

		return nil
	} else {
		return errors.New("EDU doesn't exist")
	}
}

func (d *InMemoryFederationDatabase) CleanPDUs(
	ctx context.Context,
	serverName gomatrixserverlib.ServerName,
	receipts []*receipt.Receipt,
) error {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	if pdus, ok := d.associatedPDUs[serverName]; ok {
		for _, dbReceipt := range receipts {
			delete(pdus, dbReceipt)
		}
	}

	return nil
}

func (d *InMemoryFederationDatabase) CleanEDUs(
	ctx context.Context,
	serverName gomatrixserverlib.ServerName,
	receipts []*receipt.Receipt,
) error {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	if edus, ok := d.associatedEDUs[serverName]; ok {
		for _, dbReceipt := range receipts {
			delete(edus, dbReceipt)
		}
	}

	return nil
}

func (d *InMemoryFederationDatabase) GetPendingPDUCount(
	ctx context.Context,
	serverName gomatrixserverlib.ServerName,
) (int64, error) {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	var count int64
	if pdus, ok := d.associatedPDUs[serverName]; ok {
		count = int64(len(pdus))
	}
	return count, nil
}

func (d *InMemoryFederationDatabase) GetPendingEDUCount(
	ctx context.Context,
	serverName gomatrixserverlib.ServerName,
) (int64, error) {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	var count int64
	if edus, ok := d.associatedEDUs[serverName]; ok {
		count = int64(len(edus))
	}
	return count, nil
}

func (d *InMemoryFederationDatabase) GetPendingPDUServerNames(
	ctx context.Context,
) ([]gomatrixserverlib.ServerName, error) {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	servers := []gomatrixserverlib.ServerName{}
	for server := range d.pendingPDUServers {
		servers = append(servers, server)
	}
	return servers, nil
}

func (d *InMemoryFederationDatabase) GetPendingEDUServerNames(
	ctx context.Context,
) ([]gomatrixserverlib.ServerName, error) {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	servers := []gomatrixserverlib.ServerName{}
	for server := range d.pendingEDUServers {
		servers = append(servers, server)
	}
	return servers, nil
}

func (d *InMemoryFederationDatabase) AddServerToBlacklist(
	serverName gomatrixserverlib.ServerName,
) error {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	d.blacklistedServers[serverName] = struct{}{}
	return nil
}

func (d *InMemoryFederationDatabase) RemoveServerFromBlacklist(
	serverName gomatrixserverlib.ServerName,
) error {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	delete(d.blacklistedServers, serverName)
	return nil
}

func (d *InMemoryFederationDatabase) RemoveAllServersFromBlacklist() error {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	d.blacklistedServers = make(map[gomatrixserverlib.ServerName]struct{})
	return nil
}

func (d *InMemoryFederationDatabase) IsServerBlacklisted(
	serverName gomatrixserverlib.ServerName,
) (bool, error) {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	isBlacklisted := false
	if _, ok := d.blacklistedServers[serverName]; ok {
		isBlacklisted = true
	}

	return isBlacklisted, nil
}

func (d *InMemoryFederationDatabase) SetServerAssumedOffline(
	ctx context.Context,
	serverName gomatrixserverlib.ServerName,
) error {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	d.assumedOffline[serverName] = struct{}{}
	return nil
}

func (d *InMemoryFederationDatabase) RemoveServerAssumedOffline(
	ctx context.Context,
	serverName gomatrixserverlib.ServerName,
) error {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	delete(d.assumedOffline, serverName)
	return nil
}

func (d *InMemoryFederationDatabase) RemoveAllServersAssumedOffine(
	ctx context.Context,
) error {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	d.assumedOffline = make(map[gomatrixserverlib.ServerName]struct{})
	return nil
}

func (d *InMemoryFederationDatabase) IsServerAssumedOffline(
	ctx context.Context,
	serverName gomatrixserverlib.ServerName,
) (bool, error) {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	assumedOffline := false
	if _, ok := d.assumedOffline[serverName]; ok {
		assumedOffline = true
	}

	return assumedOffline, nil
}

func (d *InMemoryFederationDatabase) P2PGetRelayServersForServer(
	ctx context.Context,
	serverName gomatrixserverlib.ServerName,
) ([]gomatrixserverlib.ServerName, error) {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	knownRelayServers := []gomatrixserverlib.ServerName{}
	if relayServers, ok := d.relayServers[serverName]; ok {
		knownRelayServers = relayServers
	}

	return knownRelayServers, nil
}

func (d *InMemoryFederationDatabase) P2PAddRelayServersForServer(
	ctx context.Context,
	serverName gomatrixserverlib.ServerName,
	relayServers []gomatrixserverlib.ServerName,
) error {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	if knownRelayServers, ok := d.relayServers[serverName]; ok {
		for _, relayServer := range relayServers {
			alreadyKnown := false
			for _, knownRelayServer := range knownRelayServers {
				if relayServer == knownRelayServer {
					alreadyKnown = true
				}
			}
			if !alreadyKnown {
				d.relayServers[serverName] = append(d.relayServers[serverName], relayServer)
			}
		}
	} else {
		d.relayServers[serverName] = relayServers
	}

	return nil
}

func (d *InMemoryFederationDatabase) FetchKeys(ctx context.Context, requests map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.Timestamp) (map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult, error) {
	return nil, nil
}

func (d *InMemoryFederationDatabase) FetcherName() string {
	return ""
}

func (d *InMemoryFederationDatabase) StoreKeys(ctx context.Context, results map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult) error {
	return nil
}

func (d *InMemoryFederationDatabase) UpdateRoom(ctx context.Context, roomID string, addHosts []types.JoinedHost, removeHosts []string, purgeRoomFirst bool) (joinedHosts []types.JoinedHost, err error) {
	return nil, nil
}

func (d *InMemoryFederationDatabase) GetJoinedHosts(ctx context.Context, roomID string) ([]types.JoinedHost, error) {
	return nil, nil
}

func (d *InMemoryFederationDatabase) GetAllJoinedHosts(ctx context.Context) ([]gomatrixserverlib.ServerName, error) {
	return nil, nil
}

func (d *InMemoryFederationDatabase) GetJoinedHostsForRooms(ctx context.Context, roomIDs []string, excludeSelf, excludeBlacklisted bool) ([]gomatrixserverlib.ServerName, error) {
	return nil, nil
}

func (d *InMemoryFederationDatabase) RemoveAllServersAssumedOffline(ctx context.Context) error {
	return nil
}

func (d *InMemoryFederationDatabase) P2PRemoveRelayServersForServer(ctx context.Context, serverName gomatrixserverlib.ServerName, relayServers []gomatrixserverlib.ServerName) error {
	return nil
}

func (d *InMemoryFederationDatabase) P2PRemoveAllRelayServersForServer(ctx context.Context, serverName gomatrixserverlib.ServerName) error {
	return nil
}

func (d *InMemoryFederationDatabase) AddOutboundPeek(ctx context.Context, serverName gomatrixserverlib.ServerName, roomID, peekID string, renewalInterval int64) error {
	return nil
}

func (d *InMemoryFederationDatabase) RenewOutboundPeek(ctx context.Context, serverName gomatrixserverlib.ServerName, roomID, peekID string, renewalInterval int64) error {
	return nil
}

func (d *InMemoryFederationDatabase) GetOutboundPeek(ctx context.Context, serverName gomatrixserverlib.ServerName, roomID, peekID string) (*types.OutboundPeek, error) {
	return nil, nil
}

func (d *InMemoryFederationDatabase) GetOutboundPeeks(ctx context.Context, roomID string) ([]types.OutboundPeek, error) {
	return nil, nil
}

func (d *InMemoryFederationDatabase) AddInboundPeek(ctx context.Context, serverName gomatrixserverlib.ServerName, roomID, peekID string, renewalInterval int64) error {
	return nil
}

func (d *InMemoryFederationDatabase) RenewInboundPeek(ctx context.Context, serverName gomatrixserverlib.ServerName, roomID, peekID string, renewalInterval int64) error {
	return nil
}

func (d *InMemoryFederationDatabase) GetInboundPeek(ctx context.Context, serverName gomatrixserverlib.ServerName, roomID, peekID string) (*types.InboundPeek, error) {
	return nil, nil
}

func (d *InMemoryFederationDatabase) GetInboundPeeks(ctx context.Context, roomID string) ([]types.InboundPeek, error) {
	return nil, nil
}

func (d *InMemoryFederationDatabase) UpdateNotaryKeys(ctx context.Context, serverName gomatrixserverlib.ServerName, serverKeys gomatrixserverlib.ServerKeys) error {
	return nil
}

func (d *InMemoryFederationDatabase) GetNotaryKeys(ctx context.Context, serverName gomatrixserverlib.ServerName, optKeyIDs []gomatrixserverlib.KeyID) ([]gomatrixserverlib.ServerKeys, error) {
	return nil, nil
}

func (d *InMemoryFederationDatabase) DeleteExpiredEDUs(ctx context.Context) error {
	return nil
}

func (d *InMemoryFederationDatabase) PurgeRoom(ctx context.Context, roomID string) error {
	return nil
}
