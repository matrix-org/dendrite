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
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/sirupsen/logrus"
)

// healthResponse is returned on requests to /api/health
type healthResponse struct {
	Code       int    `json:"code"`
	FirstError string `json:"error"`
}

// HealthCheckHandler adds a /health endpoint to the internal api mux
func HealthCheckHandler(dbConfig ...config.DatabaseOptions) http.HandlerFunc {
	conns := make([]*sql.DB, len(dbConfig))
	// populate sql connections
	for i, conf := range dbConfig {
		c, err := sqlutil.Open(&conf)
		if err != nil {
			panic(err)
		}
		conns[i] = c
	}

	return func(rw http.ResponseWriter, _ *http.Request) {
		resp := &healthResponse{
			Code:       http.StatusOK,
			FirstError: "",
		}
		err := dbPingCheck(conns, resp)
		if err != nil {
			rw.WriteHeader(resp.Code)
		}

		if err := json.NewEncoder(rw).Encode(resp); err != nil {
			logrus.WithError(err).Error("unable to encode health response")
		}
	}
}

func dbPingCheck(conns []*sql.DB, resp *healthResponse) error {
	// check every database connection
	for _, conn := range conns {
		if err := conn.Ping(); err != nil {
			resp.Code = http.StatusInternalServerError
			resp.FirstError = err.Error()
			return err
		}
	}
	return nil
}
