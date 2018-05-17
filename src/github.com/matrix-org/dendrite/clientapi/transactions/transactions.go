// Copyright 2018 New Vector Ltd
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

package transactions

import (
	"errors"
	"sync"
	"time"

	"github.com/matrix-org/util"
)

// CleanupPeriod represents time in nanoseconds after which cacheCleanService runs.
const CleanupPeriod time.Duration = 30 * time.Minute

type response struct {
	cache     *util.JSONResponse
	cacheTime time.Time
}

// Cache represents a temporary store for responses.
// This is used to ensure idempotency of requests.
type Cache struct {
	sync.RWMutex
	txns map[string]response
}

// CreateCache creates and returns a initialized Cache object.
// This cache is automatically cleaned every `CleanupPeriod`.
func CreateCache() *Cache {
	t := Cache{txns: make(map[string]response)}

	// Start cleanup service as the Cache is created
	go cacheCleanService(&t)

	return &t
}

// FetchTransaction looks up response for txnID in Cache.
// Returns a JSON response if txnID is found, which can be sent to client,
// else returns error.
func (t *Cache) FetchTransaction(txnID string) (*util.JSONResponse, error) {
	t.RLock()
	res, ok := t.txns[txnID]
	t.RUnlock()

	if ok {
		return res.cache, nil
	}

	return nil, errors.New("TxnID not present")
}

// AddTransaction adds a response for txnID in Cache for later access.
func (t *Cache) AddTransaction(txnID string, res *util.JSONResponse) {
	t.Lock()
	defer t.Unlock()
	t.txns[txnID] = response{cache: res, cacheTime: time.Now()}
}

// cacheCleanService is responsible for cleaning up transactions older than 30 min.
// It guarantees that a transaction will be present in cache for at least 30 min & at most 60 min.
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
		if t.txns[key].cacheTime.Before(expire) {
			delete(t.txns, key)
		}
		t.Unlock()
	}
}
