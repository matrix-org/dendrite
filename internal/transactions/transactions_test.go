// Copyright 2024 New Vector Ltd.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package transactions

import (
	"net/http"
	"net/url"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"

	"github.com/matrix-org/util"
)

type fakeType struct {
	ID string `json:"ID"`
}

func TestCompare(t *testing.T) {
	u1, _ := url.Parse("/send/1?accessToken=123")
	u2, _ := url.Parse("/send/1")
	c1 := CacheKey{"1", "2", filepath.Dir(u1.Path)}
	c2 := CacheKey{"1", "2", filepath.Dir(u2.Path)}
	if !reflect.DeepEqual(c1, c2) {
		t.Fatalf("Cache keys differ: %+v <> %+v", c1, c2)
	}
}

var (
	fakeAccessToken  = "aRandomAccessToken"
	fakeAccessToken2 = "anotherRandomAccessToken"
	fakeTxnID        = "aRandomTxnID"
	fakeResponse     = &util.JSONResponse{
		Code: http.StatusOK, JSON: fakeType{ID: "0"},
	}
	fakeResponse2 = &util.JSONResponse{
		Code: http.StatusOK, JSON: fakeType{ID: "1"},
	}
	fakeResponse3 = &util.JSONResponse{
		Code: http.StatusOK, JSON: fakeType{ID: "2"},
	}
)

// TestCache creates a New Cache and tests AddTransaction & FetchTransaction
func TestCache(t *testing.T) {
	fakeTxnCache := New()
	u, _ := url.Parse("")
	fakeTxnCache.AddTransaction(fakeAccessToken, fakeTxnID, u, fakeResponse)

	// Add entries for noise.
	for i := 1; i <= 100; i++ {
		fakeTxnCache.AddTransaction(
			fakeAccessToken,
			fakeTxnID+strconv.Itoa(i),
			u,
			&util.JSONResponse{Code: http.StatusOK, JSON: fakeType{ID: strconv.Itoa(i)}},
		)
	}

	testResponse, ok := fakeTxnCache.FetchTransaction(fakeAccessToken, fakeTxnID, u)
	if !ok {
		t.Error("Failed to retrieve entry for txnID: ", fakeTxnID)
	} else if testResponse.JSON != fakeResponse.JSON {
		t.Error("Fetched response incorrect. Expected: ", fakeResponse.JSON, " got: ", testResponse.JSON)
	}
}

// TestCacheScope ensures transactions with the same transaction ID are not shared
// across multiple access tokens and endpoints.
func TestCacheScope(t *testing.T) {
	cache := New()
	sendEndpoint, _ := url.Parse("/send/1?accessToken=test")
	sendToDeviceEndpoint, _ := url.Parse("/sendToDevice/1")
	cache.AddTransaction(fakeAccessToken, fakeTxnID, sendEndpoint, fakeResponse)
	cache.AddTransaction(fakeAccessToken2, fakeTxnID, sendEndpoint, fakeResponse2)
	cache.AddTransaction(fakeAccessToken2, fakeTxnID, sendToDeviceEndpoint, fakeResponse3)

	if res, ok := cache.FetchTransaction(fakeAccessToken, fakeTxnID, sendEndpoint); !ok {
		t.Errorf("failed to retrieve entry for (%s, %s)", fakeAccessToken, fakeTxnID)
	} else if res.JSON != fakeResponse.JSON {
		t.Errorf("Wrong cache entry for (%s, %s). Expected: %v; got: %v", fakeAccessToken, fakeTxnID, fakeResponse.JSON, res.JSON)
	}
	if res, ok := cache.FetchTransaction(fakeAccessToken2, fakeTxnID, sendEndpoint); !ok {
		t.Errorf("failed to retrieve entry for (%s, %s)", fakeAccessToken, fakeTxnID)
	} else if res.JSON != fakeResponse2.JSON {
		t.Errorf("Wrong cache entry for (%s, %s). Expected: %v; got: %v", fakeAccessToken, fakeTxnID, fakeResponse2.JSON, res.JSON)
	}

	// Ensure the txnID is not shared across endpoints
	if res, ok := cache.FetchTransaction(fakeAccessToken2, fakeTxnID, sendToDeviceEndpoint); !ok {
		t.Errorf("failed to retrieve entry for (%s, %s)", fakeAccessToken, fakeTxnID)
	} else if res.JSON != fakeResponse3.JSON {
		t.Errorf("Wrong cache entry for (%s, %s). Expected: %v; got: %v", fakeAccessToken, fakeTxnID, fakeResponse2.JSON, res.JSON)
	}
}
