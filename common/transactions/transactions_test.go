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
	"net/http"
	"testing"

	"github.com/matrix-org/util"
)

type fakeType struct {
	ID string `json:"ID"`
}

var (
	fakeTxnID    = "aRandomTxnID"
	fakeResponse = &util.JSONResponse{Code: http.StatusOK, JSON: fakeType{ID: "0"}}
)

// TestCache creates a New Cache and tests AddTransaction & FetchTransaction
func TestCache(t *testing.T) {
	fakeTxnCache := New()
	fakeTxnCache.AddTransaction(fakeTxnID, fakeResponse)

	// Add entries for noise.
	for i := 1; i <= 100; i++ {
		fakeTxnCache.AddTransaction(
			fakeTxnID+string(i),
			&util.JSONResponse{Code: http.StatusOK, JSON: fakeType{ID: string(i)}},
		)
	}

	testResponse, ok := fakeTxnCache.FetchTransaction(fakeTxnID)
	if !ok {
		t.Error("Failed to retrieve entry for txnID: ", fakeTxnID)
	} else if testResponse.JSON != fakeResponse.JSON {
		t.Error("Fetched response incorrect. Expected: ", fakeResponse.JSON, " got: ", testResponse.JSON)
	}
}
