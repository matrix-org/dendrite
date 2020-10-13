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

package sqlutil

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/matrix-org/dendrite/internal/config"
	"github.com/ngrok/sqlmw"
	"github.com/sirupsen/logrus"
)

var tracingEnabled = os.Getenv("DENDRITE_TRACE_SQL") == "1"
var goidToWriter sync.Map

type traceInterceptor struct {
	sqlmw.NullInterceptor
}

func (in *traceInterceptor) StmtQueryContext(ctx context.Context, stmt driver.StmtQueryContext, query string, args []driver.NamedValue) (driver.Rows, error) {
	startedAt := time.Now()
	rows, err := stmt.QueryContext(ctx, args)

	trackGoID(query)

	logrus.WithField("duration", time.Since(startedAt)).WithField(logrus.ErrorKey, err).Debug("executed sql query ", query, " args: ", args)

	return rows, err
}

func (in *traceInterceptor) StmtExecContext(ctx context.Context, stmt driver.StmtExecContext, query string, args []driver.NamedValue) (driver.Result, error) {
	startedAt := time.Now()
	result, err := stmt.ExecContext(ctx, args)

	trackGoID(query)

	logrus.WithField("duration", time.Since(startedAt)).WithField(logrus.ErrorKey, err).Debug("executed sql query ", query, " args: ", args)

	return result, err
}

func (in *traceInterceptor) RowsNext(c context.Context, rows driver.Rows, dest []driver.Value) error {
	err := rows.Next(dest)
	if err == io.EOF {
		// For all cases, we call Next() n+1 times, the first to populate the initial dest, then eventually
		// it will io.EOF. If we log on each Next() call we log the last element twice, so don't.
		return err
	}
	cols := rows.Columns()
	logrus.Debug(strings.Join(cols, " | "))

	b := strings.Builder{}
	for i, val := range dest {
		b.WriteString(fmt.Sprintf("%q", val))
		if i+1 <= len(dest)-1 {
			b.WriteString(" | ")
		}
	}
	logrus.Debug(b.String())
	return err
}

func trackGoID(query string) {
	thisGoID := goid()
	if _, ok := goidToWriter.Load(thisGoID); ok {
		return // we're on a writer goroutine
	}

	q := strings.TrimSpace(query)
	if strings.HasPrefix(q, "SELECT") {
		return // SELECTs can go on other goroutines
	}
	logrus.Warnf("unsafe goid %d: SQL executed not on an ExclusiveWriter: %s", thisGoID, q)
}

// Open opens a database specified by its database driver name and a driver-specific data source name,
// usually consisting of at least a database name and connection information. Includes tracing driver
// if DENDRITE_TRACE_SQL=1
func Open(dbProperties *config.DatabaseOptions) (*sql.DB, error) {
	var err error
	var driverName, dsn string
	switch {
	case dbProperties.ConnectionString.IsSQLite():
		driverName = SQLiteDriverName()
		dsn, err = ParseFileURI(dbProperties.ConnectionString)
		if err != nil {
			return nil, fmt.Errorf("ParseFileURI: %w", err)
		}
	case dbProperties.ConnectionString.IsPostgres():
		driverName = "postgres"
		dsn = string(dbProperties.ConnectionString)
	default:
		return nil, fmt.Errorf("invalid database connection string %q", dbProperties.ConnectionString)
	}
	if tracingEnabled {
		// install the wrapped driver
		driverName += "-trace"
	}
	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, err
	}
	if driverName != SQLiteDriverName() {
		logrus.WithFields(logrus.Fields{
			"MaxOpenConns":    dbProperties.MaxOpenConns,
			"MaxIdleConns":    dbProperties.MaxIdleConns,
			"ConnMaxLifetime": dbProperties.ConnMaxLifetime,
			"dataSourceName":  regexp.MustCompile(`://[^@]*@`).ReplaceAllLiteralString(dsn, "://"),
		}).Debug("Setting DB connection limits")
		db.SetMaxOpenConns(dbProperties.MaxOpenConns())
		db.SetMaxIdleConns(dbProperties.MaxIdleConns())
		db.SetConnMaxLifetime(dbProperties.ConnMaxLifetime())
	}
	return db, nil
}

func init() {
	registerDrivers()
}

func goid() int {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	idField := strings.Fields(strings.TrimPrefix(string(buf[:n]), "goroutine "))[0]
	id, err := strconv.Atoi(idField)
	if err != nil {
		panic(fmt.Sprintf("cannot get goroutine id: %v", err))
	}
	return id
}
