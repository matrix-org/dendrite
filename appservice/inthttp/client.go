package inthttp

import (
	"context"
	"errors"
	"net/http"

	"github.com/matrix-org/dendrite/appservice/api"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/opentracing/opentracing-go"
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
	span, ctx := opentracing.StartSpanFromContext(ctx, "appserviceRoomAliasExists")
	defer span.Finish()

	apiURL := h.appserviceURL + AppServiceRoomAliasExistsPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// UserIDExists implements AppServiceQueryAPI
func (h *httpAppServiceQueryAPI) UserIDExists(
	ctx context.Context,
	request *api.UserIDExistsRequest,
	response *api.UserIDExistsResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "appserviceUserIDExists")
	defer span.Finish()

	apiURL := h.appserviceURL + AppServiceUserIDExistsPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}
