// Copyright 2020 The Matrix.org Foundation C.I.C.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package httputil

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
)

// PostJSON performs a POST request with JSON on an internal HTTP API.
// The error will match the errtype if returned from the remote API, or
// will be a different type if there was a problem reaching the API.
func PostJSON[reqtype, restype any, errtype error](
	ctx context.Context, span opentracing.Span, httpClient *http.Client,
	apiURL string, request *reqtype, response *restype,
) error {
	jsonBytes, err := json.Marshal(request)
	if err != nil {
		return err
	}

	parsedAPIURL, err := url.Parse(apiURL)
	if err != nil {
		return err
	}

	parsedAPIURL.Path = InternalPathPrefix + strings.TrimLeft(parsedAPIURL.Path, "/")
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
	var body []byte
	body, err = io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	if res.StatusCode != http.StatusOK {
		if len(body) == 0 {
			return fmt.Errorf("HTTP %d from %s (no response body)", res.StatusCode, apiURL)
		}
		var reserr errtype
		if err = json.Unmarshal(body, &reserr); err != nil {
			return fmt.Errorf("HTTP %d from %s - %w", res.StatusCode, apiURL, err)
		}
		return reserr
	}
	if err = json.Unmarshal(body, response); err != nil {
		return fmt.Errorf("json.Unmarshal: %w", err)
	}
	return nil
}
