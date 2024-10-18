// Copyright 2024 New Vector Ltd.
// Copyright 2022 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/element-hq/dendrite/federationapi/storage/shared/receipt"
	"github.com/element-hq/dendrite/federationapi/types"
	rstypes "github.com/element-hq/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
)

var nidMutex sync.Mutex
var nid = int64(0)

type InMemoryFederationDatabase struct {
	dbMutex            sync.Mutex
	pendingPDUServers  map[spec.ServerName]struct{}
	pendingEDUServers  map[spec.ServerName]struct{}
	blacklistedServers map[spec.ServerName]struct{}
	assumedOffline     map[spec.ServerName]struct{}
	pendingPDUs        map[*receipt.Receipt]*rstypes.HeaderedEvent
	pendingEDUs        map[*receipt.Receipt]*gomatrixserverlib.EDU
	associatedPDUs     map[spec.ServerName]map[*receipt.Receipt]struct{}
	associatedEDUs     map[spec.ServerName]map[*receipt.Receipt]struct{}
	relayServers       map[spec.ServerName][]spec.ServerName
}

func NewInMemoryFederationDatabase() *InMemoryFederationDatabase {
	return &InMemoryFederationDatabase{
		pendingPDUServers:  make(map[spec.ServerName]struct{}),
		pendingEDUServers:  make(map[spec.ServerName]struct{}),
		blacklistedServers: make(map[spec.ServerName]struct{}),
		assumedOffline:     make(map[spec.ServerName]struct{}),
		pendingPDUs:        make(map[*receipt.Receipt]*rstypes.HeaderedEvent),
		pendingEDUs:        make(map[*receipt.Receipt]*gomatrixserverlib.EDU),
		associatedPDUs:     make(map[spec.ServerName]map[*receipt.Receipt]struct{}),
		associatedEDUs:     make(map[spec.ServerName]map[*receipt.Receipt]struct{}),
		relayServers:       make(map[spec.ServerName][]spec.ServerName),
	}
}

func (d *InMemoryFederationDatabase) StoreJSON(
	ctx context.Context,
	js string,
) (*receipt.Receipt, error) {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	var event rstypes.HeaderedEvent
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
	serverName spec.ServerName,
	limit int,
) (pdus map[*receipt.Receipt]*rstypes.HeaderedEvent, err error) {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	pduCount := 0
	pdus = make(map[*receipt.Receipt]*rstypes.HeaderedEvent)
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
	serverName spec.ServerName,
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
	destinations map[spec.ServerName]struct{},
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
	destinations map[spec.ServerName]struct{},
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
	serverName spec.ServerName,
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
	serverName spec.ServerName,
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
	serverName spec.ServerName,
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
	serverName spec.ServerName,
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
) ([]spec.ServerName, error) {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	servers := []spec.ServerName{}
	for server := range d.pendingPDUServers {
		servers = append(servers, server)
	}
	return servers, nil
}

func (d *InMemoryFederationDatabase) GetPendingEDUServerNames(
	ctx context.Context,
) ([]spec.ServerName, error) {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	servers := []spec.ServerName{}
	for server := range d.pendingEDUServers {
		servers = append(servers, server)
	}
	return servers, nil
}

func (d *InMemoryFederationDatabase) AddServerToBlacklist(
	serverName spec.ServerName,
) error {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	d.blacklistedServers[serverName] = struct{}{}
	return nil
}

func (d *InMemoryFederationDatabase) RemoveServerFromBlacklist(
	serverName spec.ServerName,
) error {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	delete(d.blacklistedServers, serverName)
	return nil
}

func (d *InMemoryFederationDatabase) RemoveAllServersFromBlacklist() error {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	d.blacklistedServers = make(map[spec.ServerName]struct{})
	return nil
}

func (d *InMemoryFederationDatabase) IsServerBlacklisted(
	serverName spec.ServerName,
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
	serverName spec.ServerName,
) error {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	d.assumedOffline[serverName] = struct{}{}
	return nil
}

func (d *InMemoryFederationDatabase) RemoveServerAssumedOffline(
	ctx context.Context,
	serverName spec.ServerName,
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

	d.assumedOffline = make(map[spec.ServerName]struct{})
	return nil
}

func (d *InMemoryFederationDatabase) IsServerAssumedOffline(
	ctx context.Context,
	serverName spec.ServerName,
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
	serverName spec.ServerName,
) ([]spec.ServerName, error) {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	knownRelayServers := []spec.ServerName{}
	if relayServers, ok := d.relayServers[serverName]; ok {
		knownRelayServers = relayServers
	}

	return knownRelayServers, nil
}

func (d *InMemoryFederationDatabase) P2PAddRelayServersForServer(
	ctx context.Context,
	serverName spec.ServerName,
	relayServers []spec.ServerName,
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

func (d *InMemoryFederationDatabase) P2PRemoveRelayServersForServer(
	ctx context.Context,
	serverName spec.ServerName,
	relayServers []spec.ServerName,
) error {
	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	if knownRelayServers, ok := d.relayServers[serverName]; ok {
		for _, relayServer := range relayServers {
			for i, knownRelayServer := range knownRelayServers {
				if relayServer == knownRelayServer {
					d.relayServers[serverName] = append(
						d.relayServers[serverName][:i],
						d.relayServers[serverName][i+1:]...,
					)
					break
				}
			}
		}
	} else {
		d.relayServers[serverName] = relayServers
	}

	return nil
}

func (d *InMemoryFederationDatabase) FetchKeys(ctx context.Context, requests map[gomatrixserverlib.PublicKeyLookupRequest]spec.Timestamp) (map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult, error) {
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

func (d *InMemoryFederationDatabase) GetAllJoinedHosts(ctx context.Context) ([]spec.ServerName, error) {
	return nil, nil
}

func (d *InMemoryFederationDatabase) GetJoinedHostsForRooms(ctx context.Context, roomIDs []string, excludeSelf, excludeBlacklisted bool) ([]spec.ServerName, error) {
	return nil, nil
}

func (d *InMemoryFederationDatabase) RemoveAllServersAssumedOffline(ctx context.Context) error {
	return nil
}

func (d *InMemoryFederationDatabase) P2PRemoveAllRelayServersForServer(ctx context.Context, serverName spec.ServerName) error {
	return nil
}

func (d *InMemoryFederationDatabase) AddOutboundPeek(ctx context.Context, serverName spec.ServerName, roomID, peekID string, renewalInterval int64) error {
	return nil
}

func (d *InMemoryFederationDatabase) RenewOutboundPeek(ctx context.Context, serverName spec.ServerName, roomID, peekID string, renewalInterval int64) error {
	return nil
}

func (d *InMemoryFederationDatabase) GetOutboundPeek(ctx context.Context, serverName spec.ServerName, roomID, peekID string) (*types.OutboundPeek, error) {
	return nil, nil
}

func (d *InMemoryFederationDatabase) GetOutboundPeeks(ctx context.Context, roomID string) ([]types.OutboundPeek, error) {
	return nil, nil
}

func (d *InMemoryFederationDatabase) AddInboundPeek(ctx context.Context, serverName spec.ServerName, roomID, peekID string, renewalInterval int64) error {
	return nil
}

func (d *InMemoryFederationDatabase) RenewInboundPeek(ctx context.Context, serverName spec.ServerName, roomID, peekID string, renewalInterval int64) error {
	return nil
}

func (d *InMemoryFederationDatabase) GetInboundPeek(ctx context.Context, serverName spec.ServerName, roomID, peekID string) (*types.InboundPeek, error) {
	return nil, nil
}

func (d *InMemoryFederationDatabase) GetInboundPeeks(ctx context.Context, roomID string) ([]types.InboundPeek, error) {
	return nil, nil
}

func (d *InMemoryFederationDatabase) UpdateNotaryKeys(ctx context.Context, serverName spec.ServerName, serverKeys gomatrixserverlib.ServerKeys) error {
	return nil
}

func (d *InMemoryFederationDatabase) GetNotaryKeys(ctx context.Context, serverName spec.ServerName, optKeyIDs []gomatrixserverlib.KeyID) ([]gomatrixserverlib.ServerKeys, error) {
	return nil, nil
}

func (d *InMemoryFederationDatabase) DeleteExpiredEDUs(ctx context.Context) error {
	return nil
}

func (d *InMemoryFederationDatabase) PurgeRoom(ctx context.Context, roomID string) error {
	return nil
}
