package util

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	log "github.com/sirupsen/logrus"
)

type MockJSONRequestHandler struct {
	handler func(req *http.Request) JSONResponse
}

func (h *MockJSONRequestHandler) OnIncomingRequest(req *http.Request) JSONResponse {
	return h.handler(req)
}

type MockResponse struct {
	Foo string `json:"foo"`
}

func TestMakeJSONAPI(t *testing.T) {
	log.SetLevel(log.PanicLevel) // suppress logs in test output
	tests := []struct {
		Return     JSONResponse
		ExpectCode int
		ExpectJSON string
	}{
		// MessageResponse return values
		{MessageResponse(500, "Everything is broken"), 500, `{"message":"Everything is broken"}`},
		// interface return values
		{JSONResponse{500, MockResponse{"yep"}, nil}, 500, `{"foo":"yep"}`},
		// Error JSON return values which fail to be marshalled should fallback to text
		{JSONResponse{500, struct {
			Foo interface{} `json:"foo"`
		}{func(cannotBe, marshalled string) {}}, nil}, 500, `{"message":"Internal Server Error"}`},
		// With different status codes
		{JSONResponse{201, MockResponse{"narp"}, nil}, 201, `{"foo":"narp"}`},
		// Top-level array success values
		{JSONResponse{200, []MockResponse{{"yep"}, {"narp"}}, nil}, 200, `[{"foo":"yep"},{"foo":"narp"}]`},
	}

	for _, tst := range tests {
		mock := MockJSONRequestHandler{func(req *http.Request) JSONResponse {
			return tst.Return
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

func TestMakeJSONAPICustomHeaders(t *testing.T) {
	mock := MockJSONRequestHandler{func(req *http.Request) JSONResponse {
		headers := make(map[string]string)
		headers["Custom"] = "Thing"
		headers["X-Custom"] = "Things"
		return JSONResponse{
			Code:    200,
			JSON:    MockResponse{"yep"},
			Headers: headers,
		}
	}}
	mockReq, _ := http.NewRequest("GET", "http://example.com/foo", nil)
	mockWriter := httptest.NewRecorder()
	handlerFunc := MakeJSONAPI(&mock)
	handlerFunc(mockWriter, mockReq)
	if mockWriter.Code != 200 {
		t.Errorf("TestMakeJSONAPICustomHeaders wanted HTTP status 200, got %d", mockWriter.Code)
	}
	h := mockWriter.Header().Get("Custom")
	if h != "Thing" {
		t.Errorf("TestMakeJSONAPICustomHeaders wanted header 'Custom: Thing' , got 'Custom: %s'", h)
	}
	h = mockWriter.Header().Get("X-Custom")
	if h != "Things" {
		t.Errorf("TestMakeJSONAPICustomHeaders wanted header 'X-Custom: Things' , got 'X-Custom: %s'", h)
	}
}

func TestMakeJSONAPIRedirect(t *testing.T) {
	log.SetLevel(log.PanicLevel) // suppress logs in test output
	mock := MockJSONRequestHandler{func(req *http.Request) JSONResponse {
		return RedirectResponse("https://matrix.org")
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

func TestMakeJSONAPIError(t *testing.T) {
	log.SetLevel(log.PanicLevel) // suppress logs in test output
	mock := MockJSONRequestHandler{func(req *http.Request) JSONResponse {
		err := errors.New("oops")
		return ErrorResponse(err)
	}}
	mockReq, _ := http.NewRequest("GET", "http://example.com/foo", nil)
	mockWriter := httptest.NewRecorder()
	handlerFunc := MakeJSONAPI(&mock)
	handlerFunc(mockWriter, mockReq)
	if mockWriter.Code != 500 {
		t.Errorf("TestMakeJSONAPIError wanted HTTP status 500, got %d", mockWriter.Code)
	}
	actualBody := mockWriter.Body.String()
	expect := `{"message":"oops"}`
	if actualBody != expect {
		t.Errorf("TestMakeJSONAPIError wanted body '%s', got '%s'", expect, actualBody)
	}
}

func TestIs2xx(t *testing.T) {
	tests := []struct {
		Code   int
		Expect bool
	}{
		{200, true},
		{201, true},
		{299, true},
		{300, false},
		{199, false},
		{0, false},
		{500, false},
	}
	for _, test := range tests {
		j := JSONResponse{
			Code: test.Code,
		}
		actual := j.Is2xx()
		if actual != test.Expect {
			t.Errorf("TestIs2xx wanted %t, got %t", test.Expect, actual)
		}
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
	if ctxLogger == nil {
		t.Errorf("TestGetLogger wanted logger, got nil")
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

func TestProtectWithoutLogger(t *testing.T) {
	log.SetLevel(log.PanicLevel) // suppress logs in test output
	mockWriter := httptest.NewRecorder()
	mockReq, _ := http.NewRequest("GET", "http://example.com/foo", nil)
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

func TestWithCORSOptions(t *testing.T) {
	log.SetLevel(log.PanicLevel) // suppress logs in test output
	mockWriter := httptest.NewRecorder()
	mockReq, _ := http.NewRequest("OPTIONS", "http://example.com/foo", nil)
	h := WithCORSOptions(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("yep"))
	})
	h(mockWriter, mockReq)
	if mockWriter.Code != 200 {
		t.Errorf("TestWithCORSOptions wanted HTTP status 200, got %d", mockWriter.Code)
	}

	origin := mockWriter.Header().Get("Access-Control-Allow-Origin")
	if origin != "*" {
		t.Errorf("TestWithCORSOptions wanted Access-Control-Allow-Origin header '*', got '%s'", origin)
	}

	// OPTIONS request shouldn't hit the handler func
	expectBody := ""
	actualBody := mockWriter.Body.String()
	if actualBody != expectBody {
		t.Errorf("TestWithCORSOptions wanted body %s, got %s", expectBody, actualBody)
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
