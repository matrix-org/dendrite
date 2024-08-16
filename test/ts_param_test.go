// ts_param_test.go
package testing

import (
    "net/http"
    "net/http/httptest"
    "testing"
    "time"
    "your_project_path/routing"  // Adjust this import path
)

func createRequestWithTS(ts string, isAppService bool) *http.Request {
    req := httptest.NewRequest("POST", "/_matrix/client/r0/rooms/!roomid:domain/send/m.room.message", nil)
    q := req.URL.Query()
    if ts != "" {
        q.Add("ts", ts)
    }
    req.URL.RawQuery = q.Encode()

    if isAppService {
        req.Header.Set("Authorization", "Bearer your_appservice_token")
    } else {
        req.Header.Set("Authorization", "Bearer regular_user_token")
    }
    return req
}

func TestHandleEventTimestamp_ValidAppService(t *testing.T) {
    req := createRequestWithTS("1657890000000", true)
    evTime, err := routing.HandleEventTimestamp(req)
    if err != nil {
        t.Fatalf("Expected no error, got %v", err)
    }

    expectedTime := time.Unix(1657890000, 0)
    if !evTime.Equal(expectedTime) {
        t.Errorf("Expected time %v, got %v", expectedTime, evTime)
    }
}

func TestHandleEventTimestamp_InvalidTS(t *testing.T) {
    req := createRequestWithTS("invalid_ts", true)
    _, err := routing.HandleEventTimestamp(req)
    if err == nil {
        t.Fatal("Expected an error, got none")
    }
}

func TestHandleEventTimestamp_NonAppService(t *testing.T) {
    req := createRequestWithTS("1657890000000", false)
    evTime, err := routing.HandleEventTimestamp(req)
    if err != nil {
        t.Fatalf("Expected no error, got %v", err)
    }

    if time.Now().Sub(evTime) > time.Second {
        t.Errorf("Expected current time, got %v", evTime)
    }
}
