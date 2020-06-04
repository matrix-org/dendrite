package inthttp

import (
	"context"
	"errors"
	"net/http"

	"github.com/matrix-org/dendrite/eduserver/api"
	internalHTTP "github.com/matrix-org/dendrite/internal/http"
	"github.com/opentracing/opentracing-go"
)

// HTTP paths for the internal HTTP APIs
const (
	EDUServerInputTypingEventPath       = "/eduserver/input"
	EDUServerInputSendToDeviceEventPath = "/eduserver/sendToDevice"
)

// NewEDUServerClient creates a EDUServerInputAPI implemented by talking to a HTTP POST API.
func NewEDUServerClient(eduServerURL string, httpClient *http.Client) (api.EDUServerInputAPI, error) {
	if httpClient == nil {
		return nil, errors.New("NewEDUServerClient: httpClient is <nil>")
	}
	return &httpEDUServerInputAPI{eduServerURL, httpClient}, nil
}

type httpEDUServerInputAPI struct {
	eduServerURL string
	httpClient   *http.Client
}

// InputTypingEvent implements EDUServerInputAPI
func (h *httpEDUServerInputAPI) InputTypingEvent(
	ctx context.Context,
	request *api.InputTypingEventRequest,
	response *api.InputTypingEventResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "InputTypingEvent")
	defer span.Finish()

	apiURL := h.eduServerURL + EDUServerInputTypingEventPath
	return internalHTTP.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// InputSendToDeviceEvent implements EDUServerInputAPI
func (h *httpEDUServerInputAPI) InputSendToDeviceEvent(
	ctx context.Context,
	request *api.InputSendToDeviceEventRequest,
	response *api.InputSendToDeviceEventResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "InputSendToDeviceEvent")
	defer span.Finish()

	apiURL := h.eduServerURL + EDUServerInputSendToDeviceEventPath
	return internalHTTP.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}
