package sqlutil

import (
	"database/sql"
	"fmt"
	"regexp"

	"github.com/matrix-org/dendrite/setup/config"
	"github.com/sirupsen/logrus"
)

// Open opens a database specified by its database driver name and a driver-specific data source name,
// usually consisting of at least a database name and connection information. Includes tracing driver
// if DENDRITE_TRACE_SQL=1
func Open(dbProperties *config.DatabaseOptions, writer Writer) (*sql.DB, error) {
	var err error
	var driverName, dsn string
	switch {
	case dbProperties.ConnectionString.IsSQLite():
		driverName = "sqlite3"
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
	if driverName != "sqlite3" {
		logrus.WithFields(logrus.Fields{
			"MaxOpenConns":    dbProperties.MaxOpenConns(),
			"MaxIdleConns":    dbProperties.MaxIdleConns(),
			"ConnMaxLifetime": dbProperties.ConnMaxLifetime(),
			"dataSourceName":  regexp.MustCompile(`://[^@]*@`).ReplaceAllLiteralString(dsn, "://"),
		}).Debug("Setting DB connection limits")
		db.SetMaxOpenConns(dbProperties.MaxOpenConns())
		db.SetMaxIdleConns(dbProperties.MaxIdleConns())
		db.SetConnMaxLifetime(dbProperties.ConnMaxLifetime())
	}
	return db, nil
}
