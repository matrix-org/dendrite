package util

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/test"
	"github.com/matrix-org/dendrite/test/testrig"
	"github.com/matrix-org/dendrite/userapi/storage"
)

func TestCollect(t *testing.T) {
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		b, _, _ := testrig.Base(nil)
		connStr, closeDB := test.PrepareDBConnectionString(t, dbType)
		defer closeDB()
		db, err := storage.NewUserAPIDatabase(b, &config.DatabaseOptions{
			ConnectionString: config.DataSource(connStr),
		}, "localhost", bcrypt.MinCost, 1000, 1000, "")
		if err != nil {
			t.Error(err)
		}

		receivedRequest := make(chan struct{}, 1)
		// create a test server which responds to our call
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var data map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
				t.Error(err)
			}
			defer r.Body.Close()
			if _, err := w.Write([]byte("{}")); err != nil {
				t.Error(err)
			}

			// verify the received data matches our expectations
			dbEngine, ok := data["database_engine"]
			if !ok {
				t.Errorf("missing database_engine in JSON request: %+v", data)
			}
			version, ok := data["version"]
			if !ok {
				t.Errorf("missing version in JSON request: %+v", data)
			}
			if version != internal.VersionString() {
				t.Errorf("unexpected version: %q, expected %q", version, internal.VersionString())
			}
			switch {
			case dbType == test.DBTypeSQLite && dbEngine != "SQLite":
				t.Errorf("unexpected database_engine: %s", dbEngine)
			case dbType == test.DBTypePostgres && dbEngine != "Postgres":
				t.Errorf("unexpected database_engine: %s", dbEngine)
			}
			close(receivedRequest)
		}))
		defer srv.Client()

		b.Cfg.Global.ReportStats.Endpoint = srv.URL
		stats := phoneHomeStats{
			prevData:   timestampToRUUsage{},
			serverName: "localhost",
			startTime:  time.Now(),
			cfg:        b.Cfg,
			db:         db,
			isMonolith: false,
			client:     &http.Client{Timeout: time.Second},
		}

		stats.collect()

		select {
		case <-time.After(time.Second * 5):
			t.Error("timed out waiting for response")
		case <-receivedRequest:
		}
	})
}
