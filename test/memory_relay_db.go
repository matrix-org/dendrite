// Copyright 2024 New Vector Ltd.
// Copyright 2022 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package test

import (
	"context"
	"database/sql"
	"encoding/json"
	"sync"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
)

type InMemoryRelayDatabase struct {
	nid          int64
	nidMutex     sync.Mutex
	transactions map[int64]json.RawMessage
	associations map[spec.ServerName][]int64
}

func NewInMemoryRelayDatabase() *InMemoryRelayDatabase {
	return &InMemoryRelayDatabase{
		nid:          1,
		nidMutex:     sync.Mutex{},
		transactions: make(map[int64]json.RawMessage),
		associations: make(map[spec.ServerName][]int64),
	}
}

func (d *InMemoryRelayDatabase) InsertQueueEntry(
	ctx context.Context,
	txn *sql.Tx,
	transactionID gomatrixserverlib.TransactionID,
	serverName spec.ServerName,
	nid int64,
) error {
	if _, ok := d.associations[serverName]; !ok {
		d.associations[serverName] = []int64{}
	}
	d.associations[serverName] = append(d.associations[serverName], nid)
	return nil
}

func (d *InMemoryRelayDatabase) DeleteQueueEntries(
	ctx context.Context,
	txn *sql.Tx,
	serverName spec.ServerName,
	jsonNIDs []int64,
) error {
	for _, nid := range jsonNIDs {
		for index, associatedNID := range d.associations[serverName] {
			if associatedNID == nid {
				d.associations[serverName] = append(d.associations[serverName][:index], d.associations[serverName][index+1:]...)
			}
		}
	}

	return nil
}

func (d *InMemoryRelayDatabase) SelectQueueEntries(
	ctx context.Context,
	txn *sql.Tx, serverName spec.ServerName,
	limit int,
) ([]int64, error) {
	results := []int64{}
	resultCount := limit
	if limit > len(d.associations[serverName]) {
		resultCount = len(d.associations[serverName])
	}
	if resultCount > 0 {
		for i := 0; i < resultCount; i++ {
			results = append(results, d.associations[serverName][i])
		}
	}

	return results, nil
}

func (d *InMemoryRelayDatabase) SelectQueueEntryCount(
	ctx context.Context,
	txn *sql.Tx,
	serverName spec.ServerName,
) (int64, error) {
	return int64(len(d.associations[serverName])), nil
}

func (d *InMemoryRelayDatabase) InsertQueueJSON(
	ctx context.Context,
	txn *sql.Tx,
	json string,
) (int64, error) {
	d.nidMutex.Lock()
	defer d.nidMutex.Unlock()

	nid := d.nid
	d.transactions[nid] = []byte(json)
	d.nid++

	return nid, nil
}

func (d *InMemoryRelayDatabase) DeleteQueueJSON(
	ctx context.Context,
	txn *sql.Tx,
	nids []int64,
) error {
	for _, nid := range nids {
		delete(d.transactions, nid)
	}

	return nil
}

func (d *InMemoryRelayDatabase) SelectQueueJSON(
	ctx context.Context,
	txn *sql.Tx,
	jsonNIDs []int64,
) (map[int64][]byte, error) {
	result := make(map[int64][]byte)
	for _, nid := range jsonNIDs {
		if transaction, ok := d.transactions[nid]; ok {
			result[nid] = transaction
		}
	}

	return result, nil
}
