package api

import (
	"context"

	commonHTTP "github.com/matrix-org/dendrite/common/http"

	"github.com/opentracing/opentracing-go"
)

const (
	// RoomserverPerformJoinPath is the HTTP path for the PerformJoin API.
	ServerKeyInputPublicKeyPath = "/api/serverkeyapi/inputPublicKey"

	// RoomserverPerformLeavePath is the HTTP path for the PerformLeave API.
	ServerKeyQueryPublicKeyPath = "/api/serverkeyapi/queryPublicKey"
)

type InputPublicKeysRequest struct {
}

type InputPublicKeysResponse struct {
}

func (h *httpServerKeyInternalAPI) InputPublicKeys(
	ctx context.Context,
	request *InputPublicKeysRequest,
	response *InputPublicKeysResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "InputPublicKey")
	defer span.Finish()

	apiURL := h.serverKeyAPIURL + ServerKeyInputPublicKeyPath
	return commonHTTP.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

type QueryPublicKeysRequest struct {
}

type QueryPublicKeysResponse struct {
}

func (h *httpServerKeyInternalAPI) QueryPublicKeys(
	ctx context.Context,
	request *QueryPublicKeysRequest,
	response *QueryPublicKeysResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryPublicKey")
	defer span.Finish()

	apiURL := h.serverKeyAPIURL + ServerKeyQueryPublicKeyPath
	return commonHTTP.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}
