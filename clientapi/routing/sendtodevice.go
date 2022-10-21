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

	"github.com/matrix-org/util"

	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/clientapi/producers"
	"github.com/matrix-org/dendrite/internal/transactions"
	userapi "github.com/matrix-org/dendrite/userapi/api"
)

// SendToDevice handles PUT /_matrix/client/r0/sendToDevice/{eventType}/{txnId}
// sends the device events to the syncapi & federationsender
func SendToDevice(
	req *http.Request, device *userapi.Device,
	syncProducer *producers.SyncAPIProducer,
	txnCache *transactions.Cache,
	eventType string, txnID *string,
) util.JSONResponse {
	if txnID != nil {
		if res, ok := txnCache.FetchTransaction(device.AccessToken, *txnID, req.URL); ok {
			return *res
		}
	}

	var httpReq struct {
		Messages map[string]map[string]json.RawMessage `json:"messages"`
	}
	resErr := httputil.UnmarshalJSONRequest(req, &httpReq)
	if resErr != nil {
		return *resErr
	}

	for userID, byUser := range httpReq.Messages {
		for deviceID, message := range byUser {
			if err := syncProducer.SendToDevice(
				req.Context(), device.UserID, userID, deviceID, eventType, message,
			); err != nil {
				util.GetLogger(req.Context()).WithError(err).Error("eduProducer.SendToDevice failed")
				return jsonerror.InternalServerError()
			}
		}
	}

	res := util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}

	if txnID != nil {
		txnCache.AddTransaction(device.AccessToken, *txnID, req.URL, &res)
	}

	return res
}
