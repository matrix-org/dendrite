package pushgateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestNotify(t *testing.T) {
	wantResponse := NotifyResponse{
		Rejected: []string{"testing"},
	}

	var i = 0

	svr := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// /notify only accepts POST requests
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusNotImplemented)
			return
		}

		if i != 0 { // error path
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// happy path
		json.NewEncoder(w).Encode(wantResponse)
	}))
	defer svr.Close()

	cl := NewHTTPClient(true)
	gotResponse := NotifyResponse{}

	// Test happy path
	err := cl.Notify(context.Background(), svr.URL, &NotifyRequest{}, &gotResponse)
	if err != nil {
		t.Errorf("failed to notify client")
	}
	if !reflect.DeepEqual(gotResponse, wantResponse) {
		t.Errorf("expected response %+v, got %+v", wantResponse, gotResponse)
	}

	// Test error path
	i++
	err = cl.Notify(context.Background(), svr.URL, &NotifyRequest{}, &gotResponse)
	if err == nil {
		t.Errorf("expected notifying the pushgateway to fail, but it succeeded")
	}
}
