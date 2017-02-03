package util

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	log "github.com/Sirupsen/logrus"
)

type MockJSONRequestHandler struct {
	handler func(req *http.Request) (interface{}, *HTTPError)
}

func (h *MockJSONRequestHandler) OnIncomingRequest(req *http.Request) (interface{}, *HTTPError) {
	return h.handler(req)
}

type MockResponse struct {
	Foo string `json:"foo"`
}

func TestMakeJSONAPI(t *testing.T) {
	log.SetLevel(log.PanicLevel) // suppress logs in test output
	tests := []struct {
		Return     interface{}
		Err        *HTTPError
		ExpectCode int
		ExpectJSON string
	}{
		{nil, &HTTPError{nil, "Everything is broken", 500}, 500, `{"message":"Everything is broken"}`},         // Error return values
		{nil, &HTTPError{nil, "Not here", 404}, 404, `{"message":"Not here"}`},                                 // With different status codes
		{&MockResponse{"yep"}, nil, 200, `{"foo":"yep"}`},                                                      // Success return values
		{[]MockResponse{{"yep"}, {"narp"}}, nil, 200, `[{"foo":"yep"},{"foo":"narp"}]`},                        // Top-level array success values
		{[]byte(`actually bytes`), nil, 200, `actually bytes`},                                                 // raw []byte escape hatch
		{func(cannotBe, marshalled string) {}, nil, 500, `{"message":"Failed to serialise response as JSON"}`}, // impossible marshal
	}

	for _, tst := range tests {
		mock := MockJSONRequestHandler{func(req *http.Request) (interface{}, *HTTPError) {
			return tst.Return, tst.Err
		}}
		mockReq, _ := http.NewRequest("GET", "http://example.com/foo", nil)
		mockWriter := httptest.NewRecorder()
		handlerFunc := MakeJSONAPI(&mock)
		handlerFunc(mockWriter, mockReq)
		if mockWriter.Code != tst.ExpectCode {
			t.Errorf("TestMakeJSONAPI wanted HTTP status %d, got %d", tst.ExpectCode, mockWriter.Code)
		}
		actualBody := mockWriter.Body.String()
		if actualBody != tst.ExpectJSON {
			t.Errorf("TestMakeJSONAPI wanted body '%s', got '%s'", tst.ExpectJSON, actualBody)
		}
	}
}

func TestMakeJSONAPIRedirect(t *testing.T) {
	log.SetLevel(log.PanicLevel) // suppress logs in test output
	mock := MockJSONRequestHandler{func(req *http.Request) (interface{}, *HTTPError) {
		return nil, &HTTPError{nil, "https://matrix.org", 302}
	}}
	mockReq, _ := http.NewRequest("GET", "http://example.com/foo", nil)
	mockWriter := httptest.NewRecorder()
	handlerFunc := MakeJSONAPI(&mock)
	handlerFunc(mockWriter, mockReq)
	if mockWriter.Code != 302 {
		t.Errorf("TestMakeJSONAPIRedirect wanted HTTP status 302, got %d", mockWriter.Code)
	}
	location := mockWriter.Header().Get("Location")
	if location != "https://matrix.org" {
		t.Errorf("TestMakeJSONAPIRedirect wanted Location header 'https://matrix.org', got '%s'", location)
	}
}

func TestProtect(t *testing.T) {
	log.SetLevel(log.PanicLevel) // suppress logs in test output
	mockWriter := httptest.NewRecorder()
	mockReq, _ := http.NewRequest("GET", "http://example.com/foo", nil)
	mockReq = mockReq.WithContext(
		context.WithValue(mockReq.Context(), CtxValueLogger, log.WithField("test", "yep")),
	)
	h := Protect(func(w http.ResponseWriter, req *http.Request) {
		panic("oh noes!")
	})

	h(mockWriter, mockReq)

	expectCode := 500
	if mockWriter.Code != expectCode {
		t.Errorf("TestProtect wanted HTTP status %d, got %d", expectCode, mockWriter.Code)
	}

	expectBody := `{"message":"Internal Server Error"}`
	actualBody := mockWriter.Body.String()
	if actualBody != expectBody {
		t.Errorf("TestProtect wanted body %s, got %s", expectBody, actualBody)
	}
}
