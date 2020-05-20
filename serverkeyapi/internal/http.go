package internal

import (
	"encoding/json"
	"net/http"

	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/serverkeyapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

func (s *ServerKeyAPI) SetupHTTP(servMux *http.ServeMux) {
	servMux.Handle(api.ServerKeyQueryPublicKeyPath,
		common.MakeInternalAPI("queryPublicKeys", func(req *http.Request) util.JSONResponse {
			request := api.QueryPublicKeysRequest{}
			response := api.QueryPublicKeysResponse{
				Results: map[gomatrixserverlib.ServerName]map[gomatrixserverlib.KeyID]gomatrixserverlib.PublicKeyLookupResult{},
			}
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			lookup := make(map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.Timestamp)
			for serverName, byServerName := range request.Requests {
				for keyID, timestamp := range byServerName {
					key := gomatrixserverlib.PublicKeyLookupRequest{
						ServerName: serverName,
						KeyID:      keyID,
					}
					lookup[key] = timestamp
				}
			}
			keys, err := s.DB.FetchKeys(req.Context(), lookup)
			if err != nil {
				return util.ErrorResponse(err)
			}
			for req, res := range keys {
				if _, ok := response.Results[req.ServerName]; !ok {
					response.Results[req.ServerName] = map[gomatrixserverlib.KeyID]gomatrixserverlib.PublicKeyLookupResult{}
				}
				response.Results[req.ServerName][req.KeyID] = res
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	servMux.Handle(api.ServerKeyInputPublicKeyPath,
		common.MakeInternalAPI("inputPublicKeys", func(req *http.Request) util.JSONResponse {
			request := api.InputPublicKeysRequest{}
			response := api.InputPublicKeysResponse{}
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			store := make(map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult)
			for serverName, byServerName := range request.Keys {
				for keyID, keyResult := range byServerName {
					key := gomatrixserverlib.PublicKeyLookupRequest{
						ServerName: serverName,
						KeyID:      keyID,
					}
					store[key] = keyResult
				}
			}
			if err := s.DB.StoreKeys(req.Context(), store); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
}
