package internal

import (
	"encoding/json"
	"net/http"

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/serverkeyapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

func (s *ServerKeyAPI) SetupHTTP(servMux *http.ServeMux) {
	servMux.Handle(api.ServerKeyQueryPublicKeyPath,
		internal.MakeInternalAPI("queryPublicKeys", func(req *http.Request) util.JSONResponse {
			result := map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult{}
			request := api.QueryPublicKeysRequest{}
			response := api.QueryPublicKeysResponse{}
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			lookup := make(map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.Timestamp)
			for req, timestamp := range request.Requests {
				if res, ok := s.ImmutableCache.GetServerKey(req); ok {
					result[req] = res
					continue
				}
				lookup[req] = timestamp
			}
			keys, err := s.DB.FetchKeys(req.Context(), lookup)
			if err != nil {
				return util.ErrorResponse(err)
			}
			for req, res := range keys {
				result[req] = res
			}
			response.Results = result
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	servMux.Handle(api.ServerKeyInputPublicKeyPath,
		internal.MakeInternalAPI("inputPublicKeys", func(req *http.Request) util.JSONResponse {
			request := api.InputPublicKeysRequest{}
			response := api.InputPublicKeysResponse{}
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			store := make(map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult)
			for req, res := range request.Keys {
				store[req] = res
				s.ImmutableCache.StoreServerKey(req, res)
			}
			if err := s.DB.StoreKeys(req.Context(), store); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
}
