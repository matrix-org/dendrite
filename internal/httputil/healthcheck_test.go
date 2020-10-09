// Copyright 2020 The Matrix.org Foundation C.I.C.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package httputil

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/matrix-org/dendrite/internal/config"
)

func deleteDatabase(path string) {
	err := os.Remove(path)
	if err != nil {
		fmt.Printf("failed to delete database %s: %s\n", path, err)
	}
}

func TestHealthCheckHandler(t *testing.T) {

	tests := []struct {
		name        string
		args        []config.DatabaseOptions
		want        healthResponse
		cleanUpFunc func()
		withTimeout bool
	}{
		{
			name: "without database options",
			args: []config.DatabaseOptions{},
			want: healthResponse{
				Code:       http.StatusOK,
				FirstError: "",
			},
			cleanUpFunc: func() {},
		},
		{
			name: "with database options",
			args: []config.DatabaseOptions{
				{
					ConnectionString: "file:healthcheck_test.db",
				},
				{
					ConnectionString: "file:healthcheck2_test.db",
				},
			},
			want: healthResponse{
				Code:       http.StatusOK,
				FirstError: "",
			},
			cleanUpFunc: func() {
				deleteDatabase("./healthcheck_test.db")
				deleteDatabase("./healthcheck2_test.db")
			},
		},
		{
			name: "with timeout context",
			args: []config.DatabaseOptions{
				{
					ConnectionString: "file:healthcheck3_test.db",
				},
			},
			want: healthResponse{
				Code:       http.StatusInternalServerError,
				FirstError: "context deadline exceeded",
			},
			cleanUpFunc: func() {
				deleteDatabase("./healthcheck3_test.db")
			},
			withTimeout: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler := HealthCheckHandler(tt.args...)
			defer tt.cleanUpFunc()
			req := httptest.NewRequest(http.MethodGet, "http://localhost", nil)
			if tt.withTimeout {
				ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond) // force error
				defer cancel()
				req = req.WithContext(ctx)
			}
			handler.ServeHTTP(rec, req)
			if tt.want.Code != rec.Code {
				t.Errorf("expected status code to be '%d' but got '%d'", tt.want.Code, rec.Code)
			}
			hr := healthResponse{}
			if err := json.Unmarshal(rec.Body.Bytes(), &hr); err != nil {
				t.Errorf("unable to unmarshal response '%+v'", rec.Body.Bytes())
			}
			if !reflect.DeepEqual(tt.want, hr) {
				t.Errorf("expected healthResponse to be '%+v', but got '%+v'", tt.want, hr)
			}
		})
	}
}
