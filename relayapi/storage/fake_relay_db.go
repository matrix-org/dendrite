package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"sync"

	"github.com/matrix-org/gomatrixserverlib"
)

type testDatabase struct {
	nid          int64
	nidMutex     sync.Mutex
	transactions map[int64]json.RawMessage
	associations map[gomatrixserverlib.ServerName][]int64
}

func NewFakeRelayDatabase() *testDatabase {
	return &testDatabase{
		nid:          1,
		nidMutex:     sync.Mutex{},
		transactions: make(map[int64]json.RawMessage),
		associations: make(map[gomatrixserverlib.ServerName][]int64),
	}
}

func (d *testDatabase) InsertQueueEntry(ctx context.Context, txn *sql.Tx, transactionID gomatrixserverlib.TransactionID, serverName gomatrixserverlib.ServerName, nid int64) error {
	if _, ok := d.associations[serverName]; !ok {
		d.associations[serverName] = []int64{}
	}
	d.associations[serverName] = append(d.associations[serverName], nid)
	return nil
}

func (d *testDatabase) DeleteQueueEntries(ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName, jsonNIDs []int64) error {
	for _, nid := range jsonNIDs {
		for index, associatedNID := range d.associations[serverName] {
			if associatedNID == nid {
				d.associations[serverName] = append(d.associations[serverName][:index], d.associations[serverName][index+1:]...)
			}
		}
	}

	return nil
}

func (d *testDatabase) SelectQueueEntries(ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName, limit int) ([]int64, error) {
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

func (d *testDatabase) SelectQueueEntryCount(ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName) (int64, error) {
	return int64(len(d.associations[serverName])), nil
}

func (d *testDatabase) InsertQueueJSON(ctx context.Context, txn *sql.Tx, json string) (int64, error) {
	d.nidMutex.Lock()
	defer d.nidMutex.Unlock()

	nid := d.nid
	d.transactions[nid] = []byte(json)
	d.nid++

	return nid, nil
}

func (d *testDatabase) DeleteQueueJSON(ctx context.Context, txn *sql.Tx, nids []int64) error {
	for _, nid := range nids {
		delete(d.transactions, nid)
	}

	return nil
}

func (d *testDatabase) SelectQueueJSON(ctx context.Context, txn *sql.Tx, jsonNIDs []int64) (map[int64][]byte, error) {
	result := make(map[int64][]byte)
	for _, nid := range jsonNIDs {
		if transaction, ok := d.transactions[nid]; ok {
			result[nid] = transaction
		}
	}

	return result, nil
}
