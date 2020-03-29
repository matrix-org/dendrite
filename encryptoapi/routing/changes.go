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

	"github.com/matrix-org/dendrite/encryptoapi/storage"
	"github.com/matrix-org/dendrite/encryptoapi/types"
	"github.com/matrix-org/util"
)

// ChangesInKeys returns the changes in the keys after last sync
// each user maintains a chain of the changes when provided by FED
func ChangesInKeys(
	req *http.Request,
	encryptionDB *storage.Database,
) util.JSONResponse {
	// assuming federation has added keys to the DB,
	// extracting from the DB here

	// get from FED/Req
	var readID int
	var userID string
	keyChanges, err := encryptionDB.GetKeyChanges(req.Context(), readID, userID)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: struct{}{},
		}
	}

	changesRes := types.ChangesResponse{}
	changesRes.Changed = keyChanges.Changed
	changesRes.Left = keyChanges.Left

	// delete the extracted keys from the DB

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: changesRes,
	}
}
