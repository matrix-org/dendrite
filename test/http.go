package test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"sync"
	"testing"
)

type HTTPRequestOpt func(req *http.Request)

func WithJSONBody(t *testing.T, body interface{}) HTTPRequestOpt {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("WithJSONBody: %s", err)
	}
	return func(req *http.Request) {
		req.Body = io.NopCloser(bytes.NewBuffer(b))
	}
}

func WithQueryParams(qps map[string]string) HTTPRequestOpt {
	var vals url.Values = map[string][]string{}
	for k, v := range qps {
		vals.Set(k, v)
	}
	return func(req *http.Request) {
		req.URL.RawQuery = vals.Encode()
	}
}

func NewRequest(t *testing.T, method, path string, opts ...HTTPRequestOpt) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, "http://localhost"+path, nil)
	if err != nil {
		t.Fatalf("failed to make new HTTP request %v %v : %v", method, path, err)
	}
	for _, o := range opts {
		o(req)
	}
	return req
}

// ListenAndServe will listen on a random high-numbered port and attach the given router.
// Returns the base URL to send requests to. Call `cancel` to shutdown the server, which will block until it has closed.
func ListenAndServe(t *testing.T, router http.Handler, withTLS bool) (apiURL string, cancel func()) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("failed to listen: %s", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	srv := http.Server{}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		srv.Handler = router
		var err error
		if withTLS {
			certFile := filepath.Join(t.TempDir(), "dendrite.cert")
			keyFile := filepath.Join(t.TempDir(), "dendrite.key")
			err = NewTLSKey(keyFile, certFile, 1024)
			if err != nil {
				t.Errorf("failed to make TLS key: %s", err)
				return
			}
			err = srv.ServeTLS(listener, certFile, keyFile)
		} else {
			err = srv.Serve(listener)
		}
		if err != nil && err != http.ErrServerClosed {
			t.Logf("Listen failed: %s", err)
		}
	}()
	s := ""
	if withTLS {
		s = "s"
	}
	return fmt.Sprintf("http%s://localhost:%d", s, port), func() {
		_ = srv.Shutdown(context.Background())
		wg.Wait()
	}
}
