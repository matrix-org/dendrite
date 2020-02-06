// Copyright 2019 Anton Stuetz
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

package search

import (
	"errors"
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/httputil"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	searchtypes "github.com/matrix-org/dendrite/userdirectoryapi/types"
	"github.com/matrix-org/util"

	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"
)

const (
	DEFAULT_LIMIT = 10
)

type userSearchRequest struct {
	SearchTerm string `json:"search_term"`
	Limit      int8   `json:"limit"`
}

type userSearchResponse struct {
	Results *[]searchtypes.SearchResult `json:"results"`
	Limited bool                        `json:"limited"`
}

func Search(req *http.Request, accountDB accounts.AccountDatabase) util.JSONResponse {
	var r userSearchRequest
	resErr := httputil.UnmarshalJSONRequest(req, &r)
	if resErr != nil {
		return *resErr
	}

	err := validateSearchTerm(r.SearchTerm)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.InvalidArgumentValue(err.Error()),
		}
	}
	if r.Limit == 0 {
		r.Limit = DEFAULT_LIMIT
	}
	results, limited, err := accountDB.SearchUserIdAndDisplayName(req.Context(), r.SearchTerm+"%", r.Limit)
	if err != nil {
		return util.ErrorResponse(err)
	}
	return mapProfilesToResponse(results, limited)
}

func validateSearchTerm(search_term string) error {
	if len(search_term) == 0 {
		return errors.New("Search term is empty")
	}
	return nil
}

func mapProfilesToResponse(results *[]searchtypes.SearchResult, limited bool) util.JSONResponse {
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: userSearchResponse{results, limited},
	}
}
