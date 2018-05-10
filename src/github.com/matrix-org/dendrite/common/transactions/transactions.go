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

package transactions

import (
	"errors"
	"sync"
	"time"

	"github.com/matrix-org/util"
)

// CleanupPeriod represents time in nanoseconds after which cacheCleanService runs.
const CleanupPeriod time.Duration = 30 * time.Minute

type entry struct {
	value     *util.JSONResponse
	entryTime time.Time
}

// Cache represents a temporary store for entries.
// Entries are evicted after a certain period, defined by CleanupPeriod.
type Cache struct {
	sync.RWMutex
	txns map[string]entry
}

// New creates a new Cache object, starts cacheCleanService.
// Returns a referance to newly created Cache.
func New() *Cache {
	t := Cache{txns: make(map[string]entry)}

	// Start cleanup service as the Cache is created
	go cacheCleanService(&t)
	return &t
}

// FetchTransaction looks up an entry for txnID in Cache.
// Returns a JSON response if txnID is found, else returns error.
func (t *Cache) FetchTransaction(txnID string) (*util.JSONResponse, error) {
	t.RLock()
	res, ok := t.txns[txnID]
	t.RUnlock()

	if ok {
		return res.value, nil
	}
	return nil, errors.New("TxnID not present")
}

// AddTransaction adds a entry for txnID in Cache for later access.
func (t *Cache) AddTransaction(txnID string, res *util.JSONResponse) {
	t.Lock()
	defer t.Unlock()

	t.txns[txnID] = entry{value: res, entryTime: time.Now()}
}

// cacheCleanService is responsible for cleaning up transactions older than CleanupPeriod.
// It guarantees that a transaction will be present in cache for at least CleanupPeriod & at most 2 * CleanupPeriod.
func cacheCleanService(t *Cache) {
	for {
		time.Sleep(CleanupPeriod)
		go clean(t)
	}
}

func clean(t *Cache) {
	expire := time.Now().Add(-CleanupPeriod)
	for key := range t.txns {
		t.Lock()
		if t.txns[key].entryTime.Before(expire) {
			delete(t.txns, key)
		}
		t.Unlock()
	}
}
