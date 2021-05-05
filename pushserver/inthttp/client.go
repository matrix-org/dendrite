package inthttp

import (
	"context"
	"errors"
	"net/http"

	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/pushserver/api"
	"github.com/opentracing/opentracing-go"
)

type httpPushserverInternalAPI struct {
	roomserverURL string
	httpClient    *http.Client
}

const (
	PushserverQueryExamplePath = "/pushserver/queryExample"
)

// NewRoomserverClient creates a PushserverInternalAPI implemented by talking to a HTTP POST API.
// If httpClient is nil an error is returned
func NewPushserverClient(
	pushserverURL string,
	httpClient *http.Client,
) (api.PushserverInternalAPI, error) {
	if httpClient == nil {
		return nil, errors.New("NewPushserverClient: httpClient is <nil>")
	}
	return &httpPushserverInternalAPI{
		roomserverURL: pushserverURL,
		httpClient:    httpClient,
	}, nil
}

// SetRoomAlias implements RoomserverAliasAPI
func (h *httpPushserverInternalAPI) QueryExample(
	ctx context.Context,
	request *api.QueryExampleRequest,
	response *api.QueryExampleResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryExample")
	defer span.Finish()

	apiURL := h.roomserverURL + PushserverQueryExamplePath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}
