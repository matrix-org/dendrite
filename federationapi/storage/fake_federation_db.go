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

package storage

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/matrix-org/dendrite/federationapi/storage/shared"
	"github.com/matrix-org/gomatrixserverlib"
)

var nidMutex sync.Mutex
var nid = int64(0)

type FakeFederationDatabase struct {
	Database
	dbMutex            sync.Mutex
	pendingPDUServers  map[gomatrixserverlib.ServerName]struct{}
	pendingEDUServers  map[gomatrixserverlib.ServerName]struct{}
	blacklistedServers map[gomatrixserverlib.ServerName]struct{}
	assumedOffline     map[gomatrixserverlib.ServerName]struct{}
	pendingPDUs        map[*shared.Receipt]*gomatrixserverlib.HeaderedEvent
	pendingEDUs        map[*shared.Receipt]*gomatrixserverlib.EDU
	associatedPDUs     map[gomatrixserverlib.ServerName]map[*shared.Receipt]struct{}
	associatedEDUs     map[gomatrixserverlib.ServerName]map[*shared.Receipt]struct{}
	relayServers       map[gomatrixserverlib.ServerName][]gomatrixserverlib.ServerName
}

func NewFakeFederationDatabase() *FakeFederationDatabase {
	return &FakeFederationDatabase{
		pendingPDUServers:  make(map[gomatrixserverlib.ServerName]struct{}),
		pendingEDUServers:  make(map[gomatrixserverlib.ServerName]struct{}),
		blacklistedServers: make(map[gomatrixserverlib.ServerName]struct{}),
		assumedOffline:     make(map[gomatrixserverlib.ServerName]struct{}),
		pendingPDUs:        make(map[*shared.Receipt]*gomatrixserverlib.HeaderedEvent),
		pendingEDUs:        make(map[*shared.Receipt]*gomatrixserverlib.EDU),
		associatedPDUs:     make(map[gomatrixserverlib.ServerName]map[*shared.Receipt]struct{}),
		associatedEDUs:     make(map[gomatrixserverlib.ServerName]map[*shared.Receipt]struct{}),
		relayServers:       make(map[gomatrixserverlib.ServerName][]gomatrixserverlib.ServerName),
	}
}

func (d *FakeFederationDatabase) StoreJSON(ctx context.Context, js string) (*shared.Receipt, error) {
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

func (d *FakeFederationDatabase) GetPendingPDUs(ctx context.Context, serverName gomatrixserverlib.ServerName, limit int) (pdus map[*shared.Receipt]*gomatrixserverlib.HeaderedEvent, err error) {
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

func (d *FakeFederationDatabase) GetPendingEDUs(ctx context.Context, serverName gomatrixserverlib.ServerName, limit int) (edus map[*shared.Receipt]*gomatrixserverlib.EDU, err error) {
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

func (d *FakeFederationDatabase) AssociatePDUWithDestinations(ctx context.Context, destinations map[gomatrixserverlib.ServerName]struct{}, receipt *shared.Receipt) error {
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

func (d *FakeFederationDatabase) AssociateEDUWithDestinations(ctx context.Context, destinations map[gomatrixserverlib.ServerName]struct{}, receipt *shared.Receipt, eduType string, expireEDUTypes map[string]time.Duration) error {
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

func (d *FakeFederationDatabase) CleanPDUs(ctx context.Context, serverName gomatrixserverlib.ServerName, receipts []*shared.Receipt) error {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	if pdus, ok := d.associatedPDUs[serverName]; ok {
		for _, receipt := range receipts {
			delete(pdus, receipt)
		}
	}

	return nil
}

func (d *FakeFederationDatabase) CleanEDUs(ctx context.Context, serverName gomatrixserverlib.ServerName, receipts []*shared.Receipt) error {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	if edus, ok := d.associatedEDUs[serverName]; ok {
		for _, receipt := range receipts {
			delete(edus, receipt)
		}
	}

	return nil
}

func (d *FakeFederationDatabase) GetPendingPDUCount(ctx context.Context, serverName gomatrixserverlib.ServerName) (int64, error) {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	var count int64
	if pdus, ok := d.associatedPDUs[serverName]; ok {
		count = int64(len(pdus))
	}
	return count, nil
}

func (d *FakeFederationDatabase) GetPendingEDUCount(ctx context.Context, serverName gomatrixserverlib.ServerName) (int64, error) {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	var count int64
	if edus, ok := d.associatedEDUs[serverName]; ok {
		count = int64(len(edus))
	}
	return count, nil
}

func (d *FakeFederationDatabase) GetPendingPDUServerNames(ctx context.Context) ([]gomatrixserverlib.ServerName, error) {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	servers := []gomatrixserverlib.ServerName{}
	for server := range d.pendingPDUServers {
		servers = append(servers, server)
	}
	return servers, nil
}

func (d *FakeFederationDatabase) GetPendingEDUServerNames(ctx context.Context) ([]gomatrixserverlib.ServerName, error) {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	servers := []gomatrixserverlib.ServerName{}
	for server := range d.pendingEDUServers {
		servers = append(servers, server)
	}
	return servers, nil
}

func (d *FakeFederationDatabase) AddServerToBlacklist(serverName gomatrixserverlib.ServerName) error {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	d.blacklistedServers[serverName] = struct{}{}
	return nil
}

func (d *FakeFederationDatabase) RemoveServerFromBlacklist(serverName gomatrixserverlib.ServerName) error {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	delete(d.blacklistedServers, serverName)
	return nil
}

func (d *FakeFederationDatabase) RemoveAllServersFromBlacklist() error {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	d.blacklistedServers = make(map[gomatrixserverlib.ServerName]struct{})
	return nil
}

func (d *FakeFederationDatabase) IsServerBlacklisted(serverName gomatrixserverlib.ServerName) (bool, error) {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	isBlacklisted := false
	if _, ok := d.blacklistedServers[serverName]; ok {
		isBlacklisted = true
	}

	return isBlacklisted, nil
}

func (d *FakeFederationDatabase) SetServerAssumedOffline(serverName gomatrixserverlib.ServerName) error {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	d.assumedOffline[serverName] = struct{}{}
	return nil
}

func (d *FakeFederationDatabase) RemoveServerAssumedOffline(serverName gomatrixserverlib.ServerName) error {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	delete(d.assumedOffline, serverName)
	return nil
}

func (d *FakeFederationDatabase) RemoveAllServersAssumedOffine() error {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	d.assumedOffline = make(map[gomatrixserverlib.ServerName]struct{})
	return nil
}

func (d *FakeFederationDatabase) IsServerAssumedOffline(serverName gomatrixserverlib.ServerName) (bool, error) {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	assumedOffline := false
	if _, ok := d.assumedOffline[serverName]; ok {
		assumedOffline = true
	}

	return assumedOffline, nil
}

func (d *FakeFederationDatabase) GetRelayServersForServer(serverName gomatrixserverlib.ServerName) ([]gomatrixserverlib.ServerName, error) {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	knownRelayServers := []gomatrixserverlib.ServerName{}
	if relayServers, ok := d.relayServers[serverName]; ok {
		knownRelayServers = relayServers
	}

	return knownRelayServers, nil
}

func (d *FakeFederationDatabase) AddRelayServersForServer(serverName gomatrixserverlib.ServerName, relayServers []gomatrixserverlib.ServerName) error {
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
	}

	return nil
}
