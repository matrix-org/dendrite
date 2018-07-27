// Copyright 2017 Vector Creations Ltd
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

package sync

import (
	"context"
	encryptoapi "github.com/matrix-org/dendrite/encryptoapi/storage"
	"github.com/matrix-org/dendrite/syncapi/types"
	"sync"
)

type keyCounter struct {
	sync.RWMutex
	m map[string]map[string]int
}

var counter = keyCounter{
	m: make(map[string]map[string]int),
}

// CounterRead returns uid to countMap
func CounterRead(uid string) map[string]int {
	counter.RLock()
	defer counter.RUnlock()
	return counter.m[uid]
}

// CounterWrite write count map to share for all response
func CounterWrite(uid string, m map[string]int) {
	counter.Lock()
	defer counter.Unlock()
	counter.m[uid] = m
}

// KeyCountEXT key count extension process
func KeyCountEXT(
	ctx context.Context,
	encryptionDB *encryptoapi.Database,
	respIn types.Response,
	userID, deviceID string,
) (respOut *types.Response) {

	respOut = &respIn
	// when extension works at the very beginning
	resp, err := encryptionDB.SyncOneTimeCount(ctx, userID, deviceID)
	CounterWrite(userID, resp)
	if err != nil {
		return
	}
	respOut.SignNum = resp
	return
}
