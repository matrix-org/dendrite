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
		// Error message return values
		{nil, &HTTPError{nil, "Everything is broken", 500, nil}, 500, `{"message":"Everything is broken"}`},
		// Error JSON return values
		{nil, &HTTPError{nil, "Everything is broken", 500, struct {
			Foo string `json:"foo"`
		}{"yep"}}, 500, `{"foo":"yep"}`},
		// Error JSON return values which fail to be marshalled should fallback to text
		{nil, &HTTPError{nil, "Everything is broken", 500, struct {
			Foo interface{} `json:"foo"`
		}{func(cannotBe, marshalled string) {}}}, 500, `{"message":"Everything is broken"}`},
		// With different status codes
		{nil, &HTTPError{nil, "Not here", 404, nil}, 404, `{"message":"Not here"}`},
		// Success return values
		{&MockResponse{"yep"}, nil, 200, `{"foo":"yep"}`},
		// Top-level array success values
		{[]MockResponse{{"yep"}, {"narp"}}, nil, 200, `[{"foo":"yep"},{"foo":"narp"}]`},
		// raw []byte escape hatch
		{[]byte(`actually bytes`), nil, 200, `actually bytes`},
		// impossible marshal
		{func(cannotBe, marshalled string) {}, nil, 500, `{"message":"Failed to serialise response as JSON"}`},
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
		return nil, &HTTPError{nil, "https://matrix.org", 302, nil}
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

func TestGetLogger(t *testing.T) {
	log.SetLevel(log.PanicLevel) // suppress logs in test output
	entry := log.WithField("test", "yep")
	mockReq, _ := http.NewRequest("GET", "http://example.com/foo", nil)
	ctx := context.WithValue(mockReq.Context(), ctxValueLogger, entry)
	mockReq = mockReq.WithContext(ctx)
	ctxLogger := GetLogger(mockReq.Context())
	if ctxLogger != entry {
		t.Errorf("TestGetLogger wanted logger '%v', got '%v'", entry, ctxLogger)
	}

	noLoggerInReq, _ := http.NewRequest("GET", "http://example.com/foo", nil)
	ctxLogger = GetLogger(noLoggerInReq.Context())
	if ctxLogger != nil {
		t.Errorf("TestGetLogger wanted nil logger, got '%v'", ctxLogger)
	}
}

func TestProtect(t *testing.T) {
	log.SetLevel(log.PanicLevel) // suppress logs in test output
	mockWriter := httptest.NewRecorder()
	mockReq, _ := http.NewRequest("GET", "http://example.com/foo", nil)
	mockReq = mockReq.WithContext(
		context.WithValue(mockReq.Context(), ctxValueLogger, log.WithField("test", "yep")),
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

func TestGetRequestID(t *testing.T) {
	log.SetLevel(log.PanicLevel) // suppress logs in test output
	reqID := "alphabetsoup"
	mockReq, _ := http.NewRequest("GET", "http://example.com/foo", nil)
	ctx := context.WithValue(mockReq.Context(), ctxValueRequestID, reqID)
	mockReq = mockReq.WithContext(ctx)
	ctxReqID := GetRequestID(mockReq.Context())
	if reqID != ctxReqID {
		t.Errorf("TestGetRequestID wanted request ID '%s', got '%s'", reqID, ctxReqID)
	}

	noReqIDInReq, _ := http.NewRequest("GET", "http://example.com/foo", nil)
	ctxReqID = GetRequestID(noReqIDInReq.Context())
	if ctxReqID != "" {
		t.Errorf("TestGetRequestID wanted empty request ID, got '%s'", ctxReqID)
	}
}
