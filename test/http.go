package test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
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
