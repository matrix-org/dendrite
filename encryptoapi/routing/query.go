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

	"github.com/matrix-org/dendrite/clientapi/auth/storage/devices"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/encryptoapi/storage"
	"github.com/matrix-org/dendrite/encryptoapi/types"
	"github.com/matrix-org/util"
)

// QueryPKeys returns the public identity keys
// and supported algorithms of "intended user"
// This just forwards the request to the Federation,
// and waits/checks for timeouts and failures. Response
// of the FedSenderAPI is bundled with the failures and returned.
func QueryPKeys(
	req *http.Request,
	encryptionDB *storage.Database,
	deviceID string,
	deviceDB *devices.Database,
) util.JSONResponse {
	var queryRq types.QueryRequest
	if reqErr := httputil.UnmarshalJSONRequest(req, &queryRq); reqErr != nil {
		return *reqErr
	}
	queryRp := types.QueryResponse{}

	// sendDKToFed := queryRq.DeviceKeys

	var obtainedFromFed types.QueryResponse
	obtainedKeysFromFed := obtainedFromFed.DeviceKeys
	queryRp.DeviceKeys = obtainedKeysFromFed

	queryRp.Failure = make(map[string]interface{})
	// FED must return the keys from the other user
	/*
		federation consideration: when user id is in federation, a
		query is needed to ask fed for keys.
		domain --------+ fed (keys)
		domain +--tout-- timer
	*/
	// todo: Add federation processing at specific userID.
	if false /*federation judgement*/ {
		tout := queryRq.Timeout
		if tout == 0 {
			tout = int64(10 * time.Second)
		}
		stimuCh := make(chan int)
		go func() {
			time.Sleep(time.Duration(tout) * 1000 * 1000)
			close(stimuCh)
		}()
		select {
		case <-stimuCh:
			queryRp.Failure = make(map[string]interface{})
			// todo: key in this map is restricted to username at the end, yet a mocked one.
			queryRp.Failure["@alice:localhost"] = "ran out of offered time"
		case <-make(chan interface{}):
			// todo : here goes federation chan , still a mocked one
		}
		// probably some other better error to tell it timed out in FED
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: struct{}{},
		}
	}

	//
	//
	//
	//
	// Forward the request to the federation server and get the required info

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: queryRp,
	}
}
