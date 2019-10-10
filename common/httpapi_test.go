package common

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWrapHandlerInBasicAuth(t *testing.T) {
	type args struct {
		h http.Handler
		b BasicAuth
	}

	dummyHandler := http.HandlerFunc(func(h http.ResponseWriter, r *http.Request) {
		h.WriteHeader(http.StatusOK)
	})

	tests := []struct {
		name string
		args args
		want int
	}{
		{
			name: "no user or password setup",
			args: args{h: dummyHandler},
			want: http.StatusOK,
		},
		{
			name: "only user set",
			args: args{
				h: dummyHandler,
				b: BasicAuth{Username: "test"}, // no basic auth
			},
			want: http.StatusOK,
		},
		{
			name: "only pass set",
			args: args{
				h: dummyHandler,
				b: BasicAuth{Password: "test"}, // no basic auth
			},
			want: http.StatusOK,
		},
		{
			name: "credentials correct",
			args: args{
				h: dummyHandler,
				b: BasicAuth{Username: "test", Password: "test"}, // basic auth enabled
			},
			want: http.StatusOK,
		},
		{
			name: "credentials wrong",
			args: args{
				h: dummyHandler,
				b: BasicAuth{Username: "test1", Password: "test"}, // basic auth enabled
			},
			want: http.StatusForbidden,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baHandler := WrapHandlerInBasicAuth(tt.args.h, tt.args.b)

			req := httptest.NewRequest("GET", "http://localhost/metrics", nil)
			req.SetBasicAuth("test", "test")

			w := httptest.NewRecorder()
			baHandler(w, req)
			resp := w.Result()

			if resp.StatusCode != tt.want {
				t.Errorf("Expected status code %d, got %d", resp.StatusCode, tt.want)
			}
		})
	}
}
