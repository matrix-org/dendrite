package inthttp

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/serverkeyapi/api"
	"github.com/matrix-org/util"
)

func AddRoutes(s api.ServerKeyInternalAPI, internalAPIMux *mux.Router, cache caching.ServerKeyCache) {
	internalAPIMux.Handle(ServerKeyQueryLocalKeysPath,
		internal.MakeInternalAPI("queryLocalKeys", func(req *http.Request) util.JSONResponse {
			request := api.QueryLocalKeysRequest{}
			response := api.QueryLocalKeysResponse{}
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := s.QueryLocalKeys(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(ServerKeyQueryPublicKeyPath,
		internal.MakeInternalAPI("queryPublicKeys", func(req *http.Request) util.JSONResponse {
			request := api.QueryPublicKeysRequest{}
			response := api.QueryPublicKeysResponse{}
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			keys, err := s.FetchKeys(req.Context(), request.Requests)
			if err != nil {
				return util.ErrorResponse(err)
			}
			response.Results = keys
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(ServerKeyInputPublicKeyPath,
		internal.MakeInternalAPI("inputPublicKeys", func(req *http.Request) util.JSONResponse {
			request := api.InputPublicKeysRequest{}
			response := api.InputPublicKeysResponse{}
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := s.StoreKeys(req.Context(), request.Keys); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
}
