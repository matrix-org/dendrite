package pushgateway

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/element-hq/dendrite/internal"
)

type httpClient struct {
	hc *http.Client
}

// NewHTTPClient creates a new Push Gateway client.
func NewHTTPClient(disableTLSValidation bool) Client {
	hc := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			DisableKeepAlives: true,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: disableTLSValidation,
			},
			Proxy: http.ProxyFromEnvironment,
		},
	}
	return &httpClient{hc: hc}
}

func (h *httpClient) Notify(ctx context.Context, url string, req *NotifyRequest, resp *NotifyResponse) error {
	trace, ctx := internal.StartRegion(ctx, "Notify")
	defer trace.EndRegion()

	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	hreq.Header.Set("Content-Type", "application/json")

	hresp, err := h.hc.Do(hreq)
	if err != nil {
		return err
	}

	defer internal.CloseAndLogIfError(ctx, hresp.Body, "failed to close response body")

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
