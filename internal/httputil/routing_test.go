package httputil

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestRoutersError(t *testing.T) {
	r := NewRouters()

	// not found test
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, filepath.Join(PublicFederationPathPrefix, "doesnotexist"), nil)
	r.Federation.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unexpected status code: %d - %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Result().Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("unexpected content-type: %s", ct)
	}

	// not allowed test
	r.DendriteAdmin.
		Handle("/test", http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {})).
		Methods(http.MethodPost)

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, filepath.Join(DendriteAdminPathPrefix, "test"), nil)
	r.DendriteAdmin.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("unexpected status code: %d - %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Result().Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("unexpected content-type: %s", ct)
	}
}
