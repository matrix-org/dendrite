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
	"sync"
	"time"

	"github.com/matrix-org/util"
)

// DefaultCleanupPeriod represents the default time duration after which cacheCleanService runs.
const DefaultCleanupPeriod time.Duration = 30 * time.Minute

type txnsMap map[string]*util.JSONResponse

// Cache represents a temporary store for response entries.
// Entries are evicted after a certain period, defined by cleanupPeriod.
type Cache struct {
	sync.RWMutex
	txnsMaps      [2]txnsMap
	cleanupPeriod time.Duration
}

// New creates a new Cache object, starts cacheCleanService.
// Takes cleanupPeriod (in minutes) as argument, in case of 0 DefaultCleanupPeriod is used instead.
// Returns a reference to newly created Cache.
func New(cleanupPeriodInMins int) *Cache {
	t := Cache{txnsMaps: [2]txnsMap{make(txnsMap), make(txnsMap)}}

	cleanupPeriod := time.Duration(cleanupPeriodInMins) * time.Minute
	if cleanupPeriod == 0 {
		cleanupPeriod = DefaultCleanupPeriod
	}
	t.cleanupPeriod = cleanupPeriod

	// Start clean service as the Cache is created
	go cacheCleanService(&t)
	return &t
}

// FetchTransaction looks up an entry for txnID in Cache.
// Looks in both the txnMaps.
// Returns (JSON response, true) if txnID is found, else the returned bool is false.
func (t *Cache) FetchTransaction(txnID string) (*util.JSONResponse, bool) {
	t.RLock()
	defer t.RUnlock()
	for _, txns := range t.txnsMaps {
		res, ok := txns[txnID]
		if ok {
			return res, true
		}
	}
	return nil, false
}

// AddTransaction adds an entry for txnID in Cache for later access.
// Adds to the front txnMap.
func (t *Cache) AddTransaction(txnID string, res *util.JSONResponse) {
	t.Lock()
	defer t.Unlock()

	t.txnsMaps[0][txnID] = res
}

// cacheCleanService is responsible for cleaning up entries after cleanupPeriod.
// It guarantees that an entry will be present in cache for at least cleanupPeriod & at most 2 * cleanupPeriod.
func cacheCleanService(t *Cache) {
	ticker := time.Tick(t.cleanupPeriod)
	for range ticker {
		t.Lock()
		t.txnsMaps[1] = t.txnsMaps[0]
		t.txnsMaps[0] = make(txnsMap)
		t.Unlock()
	}
}
