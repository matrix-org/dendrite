package inthttp

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/eduserver/api"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/util"
)

// AddRoutes adds the EDUServerInputAPI handlers to the http.ServeMux.
func AddRoutes(t api.EDUServerInputAPI, internalAPIMux *mux.Router) {
	internalAPIMux.Handle(EDUServerInputTypingEventPath,
		httputil.MakeInternalAPI("inputTypingEvents", func(req *http.Request) util.JSONResponse {
			var request api.InputTypingEventRequest
			var response api.InputTypingEventResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := t.InputTypingEvent(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(EDUServerInputSendToDeviceEventPath,
		httputil.MakeInternalAPI("inputSendToDeviceEvents", func(req *http.Request) util.JSONResponse {
			var request api.InputSendToDeviceEventRequest
			var response api.InputSendToDeviceEventResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := t.InputSendToDeviceEvent(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(EDUServerInputReceiptEventPath,
		httputil.MakeInternalAPI("inputReceiptEvent", func(req *http.Request) util.JSONResponse {
			var request api.InputReceiptEventRequest
			var response api.InputReceiptEventResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := t.InputReceiptEvent(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
}
