package inthttp

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/appservice/api"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/util"
)

// AddRoutes adds the AppServiceQueryAPI handlers to the http.ServeMux.
func AddRoutes(a api.AppServiceInternalAPI, internalAPIMux *mux.Router) {
	internalAPIMux.Handle(
		AppServiceRoomAliasExistsPath,
		httputil.MakeInternalAPI("appserviceRoomAliasExists", func(req *http.Request) util.JSONResponse {
			var request api.RoomAliasExistsRequest
			var response api.RoomAliasExistsResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := a.RoomAliasExists(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(
		AppServiceUserIDExistsPath,
		httputil.MakeInternalAPI("appserviceUserIDExists", func(req *http.Request) util.JSONResponse {
			var request api.UserIDExistsRequest
			var response api.UserIDExistsResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := a.UserIDExists(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
}
