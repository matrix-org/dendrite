package internal

import (
	"encoding/json"
	"net/http"

	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/serverkeyapi/api"
	"github.com/matrix-org/util"
)

func (s *ServerKeyAPI) SetupHTTP(servMux *http.ServeMux) {
	servMux.Handle(api.ServerKeyQueryPublicKeyPath,
		common.MakeInternalAPI("queryPublicKeys", func(req *http.Request) util.JSONResponse {
			var request api.QueryPublicKeysRequest
			var response api.QueryPublicKeysResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			/*
				if err := s.DB.FetchKeys(); err != nil {
					return util.ErrorResponse(err)
				}
			*/
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	servMux.Handle(api.ServerKeyInputPublicKeyPath,
		common.MakeInternalAPI("inputPublicKeys", func(req *http.Request) util.JSONResponse {
			var request api.InputPublicKeysRequest
			var response api.InputPublicKeysResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			/*
				if err := s.DB.FetchKeys(); err != nil {
					return util.ErrorResponse(err)
				}
			*/
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
}
