package inthttp

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/pushserver/api"
	"github.com/matrix-org/util"
)

// AddRoutes adds the PushserverInternalAPI handlers to the http.ServeMux.
// nolint: gocyclo
func AddRoutes(r api.PushserverInternalAPI, internalAPIMux *mux.Router) {

	internalAPIMux.Handle(PerformPusherSetPath,
		httputil.MakeInternalAPI("performPusherSet", func(req *http.Request) util.JSONResponse {
			request := api.PerformPusherSetRequest{}
			response := struct{}{}
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := r.PerformPusherSet(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(PerformPusherDeletionPath,
		httputil.MakeInternalAPI("performPusherDeletion", func(req *http.Request) util.JSONResponse {
			request := api.PerformPusherDeletionRequest{}
			response := struct{}{}
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := r.PerformPusherDeletion(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)

}
