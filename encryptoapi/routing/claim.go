// Copyright 2019 Sumukha PK
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

package routing

import (
	"net/http"
	"time"

	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/encryptoapi/storage"
	"github.com/matrix-org/dendrite/encryptoapi/types"
	"github.com/matrix-org/util"
)

// ClaimOneTimeKeys enables user to claim one time keys for sessions.
func ClaimOneTimeKeys(
	req *http.Request,
	encryptionDB *storage.Database,
) util.JSONResponse {
	var claimRq types.ClaimRequest
	claimRes := types.ClaimResponse{}
	claimRes.Failures = make(map[string]interface{})
	claimRes.OneTimeKeys = make(map[string]map[string]map[string]interface{})
	if reqErr := httputil.UnmarshalJSONRequest(req, &claimRq); reqErr != nil {
		return *reqErr
	}

	var obtainedFromFed types.QueryResponse
	obtainedKeysFromFed := obtainedFromFed.DeviceKeys
	claimRes.OneTimeKeys = obtainedKeysFromFed

	// not sure what FED should return here
	/*
		federation consideration: when user id is in federation, a query is needed to ask fed for keys
		domain --------+ fed (keys)
		domain +--tout-- timer
	*/
	// todo: Add federation processing at specific userID.
	if false /*federation judgement*/ {
		tout := claimRq.Timeout
		stimuCh := make(chan int)
		go func() {
			time.Sleep(time.Duration(tout) * 1000 * 1000)
			close(stimuCh)
		}()
		select {
		case <-stimuCh:
			claimRes.Failures = make(map[string]interface{})
			// todo: key in this map is restricted to username at the end, yet a mocked one.
			claimRes.Failures["@alice:localhost"] = "ran out of offered time"
		case <-make(chan interface{}):
			// todo : here goes federation chan , still a mocked one
		}
		// probably some other better error to tell it timed out in FED
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: struct{}{},
		}
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: claimRes,
	}
}
