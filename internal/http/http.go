package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/matrix-org/dendrite/internal/httpapis"
	opentracing "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
)

// PostJSON performs a POST request with JSON on an internal HTTP API
func PostJSON(
	ctx context.Context, span opentracing.Span, httpClient *http.Client,
	apiURL string, request, response interface{},
) error {
	jsonBytes, err := json.Marshal(request)
	if err != nil {
		return err
	}

	parsedAPIURL, err := url.Parse(apiURL)
	if err != nil {
		return err
	}

	parsedAPIURL.Path = httpapis.InternalPathPrefix + strings.TrimLeft(parsedAPIURL.Path, "/")
	apiURL = parsedAPIURL.String()

	req, err := http.NewRequest(http.MethodPost, apiURL, bytes.NewReader(jsonBytes))
	if err != nil {
		return err
	}

	// Mark the span as being an RPC client.
	ext.SpanKindRPCClient.Set(span)
	carrier := opentracing.HTTPHeadersCarrier(req.Header)
	tracer := opentracing.GlobalTracer()

	if err = tracer.Inject(span.Context(), opentracing.HTTPHeaders, carrier); err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	res, err := httpClient.Do(req.WithContext(ctx))
	if res != nil {
		defer (func() { err = res.Body.Close() })()
	}
	if err != nil {
		return err
	}
	if res.StatusCode != http.StatusOK {
		var errorBody struct {
			Message string `json:"message"`
		}
		if msgerr := json.NewDecoder(res.Body).Decode(&errorBody); msgerr == nil {
			return fmt.Errorf("Internal API: %d from %s: %s", res.StatusCode, apiURL, errorBody.Message)
		}
		return fmt.Errorf("Internal API: %d from %s", res.StatusCode, apiURL)
	}
	return json.NewDecoder(res.Body).Decode(response)
}
