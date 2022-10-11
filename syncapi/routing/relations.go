// Copyright 2022 The Matrix.org Foundation C.I.C.
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

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/userapi/api"
)

type RelationsResponse struct {
	Chunk     []gomatrixserverlib.ClientEvent `json:"chunk"`
	NextBatch string                          `json:"next_batch,omitempty"`
	PrevBatch string                          `json:"prev_batch,omitempty"`
}

// nolint:gocyclo
func Relations(req *http.Request, device *api.Device, syncDB storage.Database, roomID, eventID, relType, eventType string) util.JSONResponse {
	dir := req.URL.Query().Get("dir")
	from := req.URL.Query().Get("from")
	to := req.URL.Query().Get("to")
	limit := req.URL.Query().Get("limit")

	if dir != "b" && dir != "f" {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.MissingArgument("Bad or missing dir query parameter (should be either 'b' or 'f')"),
		}
	}
	if dir == "" {
		dir = "b"
	}

	res := &RelationsResponse{}

	snapshot, err := syncDB.NewDatabaseSnapshot(req.Context())
	if err != nil {
		return jsonerror.InternalServerError()
	}
	var succeeded bool
	defer sqlutil.EndTransactionWithCheck(snapshot, &succeeded, &err)

	_, _, _, _ = from, to, limit, dir

	succeeded = true
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: res,
	}
}
