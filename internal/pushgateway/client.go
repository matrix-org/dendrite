package pushgateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/opentracing/opentracing-go"
)

type httpClient struct {
	hc *gomatrixserverlib.Client
}

// NewHTTPClient creates a new Push Gateway client.
func NewHTTPClient(disableTLSValidation bool) Client {
	return &httpClient{
		hc: gomatrixserverlib.NewClient(
			gomatrixserverlib.WithTimeout(time.Second*30),
			gomatrixserverlib.WithKeepAlives(false),
			gomatrixserverlib.WithSkipVerify(disableTLSValidation),
		),
	}
}

func (h *httpClient) Notify(ctx context.Context, url string, req *NotifyRequest, resp *NotifyResponse) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "Notify")
	defer span.Finish()

	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	hreq.Header.Set("Content-Type", "application/json")

	hresp, err := h.hc.DoHTTPRequest(ctx, hreq)
	if err != nil {
		return err
	}

	//nolint:errcheck
	defer hresp.Body.Close()

	if hresp.StatusCode == http.StatusOK {
		return json.NewDecoder(hresp.Body).Decode(resp)
	}

	var errorBody struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(hresp.Body).Decode(&errorBody); err == nil {
		return fmt.Errorf("push gateway: %d from %s: %s", hresp.StatusCode, url, errorBody.Message)
	}
	return fmt.Errorf("push gateway: %d from %s", hresp.StatusCode, url)
}
