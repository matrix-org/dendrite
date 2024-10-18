// Copyright 2024 New Vector Ltd.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package transactions

import (
	"net/url"
	"path/filepath"
	"sync"
	"time"

	"github.com/matrix-org/util"
)

// DefaultCleanupPeriod represents the default time duration after which cacheCleanService runs.
const DefaultCleanupPeriod time.Duration = 30 * time.Minute

type txnsMap map[CacheKey]*util.JSONResponse

// CacheKey is the type for the key in a transactions cache.
// This is needed because the spec requires transaction IDs to have a per-access token scope.
type CacheKey struct {
	AccessToken string
	TxnID       string
	Endpoint    string
}

// Cache represents a temporary store for response entries.
// Entries are evicted after a certain period, defined by cleanupPeriod.
// This works by keeping two maps of entries, and cycling the maps after the cleanupPeriod.
type Cache struct {
	sync.RWMutex
	txnsMaps      [2]txnsMap
	cleanupPeriod time.Duration
}

// New is a wrapper which calls NewWithCleanupPeriod with DefaultCleanupPeriod as argument.
func New() *Cache {
	return NewWithCleanupPeriod(DefaultCleanupPeriod)
}

// NewWithCleanupPeriod creates a new Cache object, starts cacheCleanService.
// Takes cleanupPeriod as argument.
// Returns a reference to newly created Cache.
func NewWithCleanupPeriod(cleanupPeriod time.Duration) *Cache {
	t := Cache{txnsMaps: [2]txnsMap{make(txnsMap), make(txnsMap)}}
	t.cleanupPeriod = cleanupPeriod

	// Start clean service as the Cache is created
	go cacheCleanService(&t)
	return &t
}

// FetchTransaction looks up an entry for the (accessToken, txnID, req.URL) tuple in Cache.
// Looks in both the txnMaps.
// Returns (JSON response, true) if txnID is found, else the returned bool is false.
func (t *Cache) FetchTransaction(accessToken, txnID string, u *url.URL) (*util.JSONResponse, bool) {
	t.RLock()
	defer t.RUnlock()
	for _, txns := range t.txnsMaps {
		res, ok := txns[CacheKey{accessToken, txnID, filepath.Dir(u.Path)}]
		if ok {
			return res, true
		}
	}
	return nil, false
}

// AddTransaction adds an entry for the (accessToken, txnID, req.URL) tuple in Cache.
// Adds to the front txnMap.
func (t *Cache) AddTransaction(accessToken, txnID string, u *url.URL, res *util.JSONResponse) {
	t.Lock()
	defer t.Unlock()
	t.txnsMaps[0][CacheKey{accessToken, txnID, filepath.Dir(u.Path)}] = res
}

// cacheCleanService is responsible for cleaning up entries after cleanupPeriod.
// It guarantees that an entry will be present in cache for at least cleanupPeriod & at most 2 * cleanupPeriod.
// This cycles the txnMaps forward, i.e. back map is assigned the front and front is assigned an empty map.
func cacheCleanService(t *Cache) {
	ticker := time.NewTicker(t.cleanupPeriod).C
	for range ticker {
		t.Lock()
		t.txnsMaps[1] = t.txnsMaps[0]
		t.txnsMaps[0] = make(txnsMap)
		t.Unlock()
	}
}
