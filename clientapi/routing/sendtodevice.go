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
	"encoding/json"
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/clientapi/producers"
	"github.com/matrix-org/dendrite/internal/transactions"
	"github.com/matrix-org/util"
)

// SendToDevice handles PUT /_matrix/client/r0/sendToDevice/{eventType}/{txnId}
// sends the device events to the EDU Server
func SendToDevice(
	req *http.Request, device *authtypes.Device,
	eduProducer *producers.EDUServerProducer,
	txnCache *transactions.Cache,
	eventType string, txnID *string,
) util.JSONResponse {
	if txnID != nil {
		// Try to fetch response from transactionsCache
		if res, ok := txnCache.FetchTransaction(device.AccessToken, *txnID); ok {
			return *res
		}
	}

	// parse the incoming http request
	var httpReq struct {
		Messages map[string]map[string]json.RawMessage `json:"messages"`
	}
	resErr := httputil.UnmarshalJSONRequest(req, &req)
	if resErr != nil {
		return *resErr
	}

	for userID, byUser := range httpReq.Messages {
		for deviceID, message := range byUser {
			if err := eduProducer.SendToDevice(
				req.Context(), userID, deviceID, eventType, message,
			); err != nil {
				util.GetLogger(req.Context()).WithError(err).Error("eduProducer.SendToDevice failed")
				return jsonerror.InternalServerError()
			}
		}
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}
