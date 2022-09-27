package inthttp

import (
	"context"
	"errors"
	"net/http"

	"github.com/matrix-org/dendrite/appservice/api"
	"github.com/matrix-org/dendrite/internal/httputil"
)

// HTTP paths for the internal HTTP APIs
const (
	AppServiceRoomAliasExistsPath = "/appservice/RoomAliasExists"
	AppServiceUserIDExistsPath    = "/appservice/UserIDExists"
)

// httpAppServiceQueryAPI contains the URL to an appservice query API and a
// reference to a httpClient used to reach it
type httpAppServiceQueryAPI struct {
	appserviceURL string
	httpClient    *http.Client
}

// NewAppserviceClient creates a AppServiceQueryAPI implemented by talking
// to a HTTP POST API.
// If httpClient is nil an error is returned
func NewAppserviceClient(
	appserviceURL string,
	httpClient *http.Client,
) (api.AppServiceInternalAPI, error) {
	if httpClient == nil {
		return nil, errors.New("NewRoomserverAliasAPIHTTP: httpClient is <nil>")
	}
	return &httpAppServiceQueryAPI{appserviceURL, httpClient}, nil
}

// RoomAliasExists implements AppServiceQueryAPI
func (h *httpAppServiceQueryAPI) RoomAliasExists(
	ctx context.Context,
	request *api.RoomAliasExistsRequest,
	response *api.RoomAliasExistsResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"RoomAliasExists", h.appserviceURL+AppServiceRoomAliasExistsPath,
		h.httpClient, ctx, request, response,
	)
}

// UserIDExists implements AppServiceQueryAPI
func (h *httpAppServiceQueryAPI) UserIDExists(
	ctx context.Context,
	request *api.UserIDExistsRequest,
	response *api.UserIDExistsResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"UserIDExists", h.appserviceURL+AppServiceUserIDExistsPath,
		h.httpClient, ctx, request, response,
	)
}
