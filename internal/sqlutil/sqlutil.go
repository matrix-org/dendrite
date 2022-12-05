package sqlutil

import (
	"database/sql"
	"flag"
	"fmt"
	"regexp"

	"github.com/matrix-org/dendrite/setup/config"
	"github.com/sirupsen/logrus"
)

var skipSanityChecks = flag.Bool("skip-db-sanity", false, "Ignore sanity checks on the database connections (NOT RECOMMENDED!)")

// Open opens a database specified by its database driver name and a driver-specific data source name,
// usually consisting of at least a database name and connection information. Includes tracing driver
// if DENDRITE_TRACE_SQL=1
func Open(dbProperties *config.DatabaseOptions, writer Writer) (*sql.DB, error) {
	var err error
	var driverName, dsn string
	switch {
	case dbProperties.ConnectionString.IsSQLite():
		driverName = SQLITE_DRIVER_NAME
		dsn, err = ParseFileURI(dbProperties.ConnectionString)
		if err != nil {
			return nil, fmt.Errorf("ParseFileURI: %w", err)
		}
		dsn = sqliteDSNExtension(dsn)
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
	if driverName != SQLITE_DRIVER_NAME {
		logger := logrus.WithFields(logrus.Fields{
			"max_open_conns":    dbProperties.MaxOpenConns(),
			"max_idle_conns":    dbProperties.MaxIdleConns(),
			"conn_max_lifetime": dbProperties.ConnMaxLifetime(),
			"data_source_name":  regexp.MustCompile(`://[^@]*@`).ReplaceAllLiteralString(dsn, "://"),
		})
		logger.Debug("Setting DB connection limits")
		db.SetMaxOpenConns(dbProperties.MaxOpenConns())
		db.SetMaxIdleConns(dbProperties.MaxIdleConns())
		db.SetConnMaxLifetime(dbProperties.ConnMaxLifetime())

		if !*skipSanityChecks {
			if dbProperties.MaxOpenConns() == 0 {
				logrus.Warnf("WARNING: Configuring 'max_open_conns' to be unlimited is not recommended. This can result in bad performance or deadlocks.")
			}

			switch driverName {
			case "postgres":
				// Perform a quick sanity check if possible that we aren't trying to use more database
				// connections than PostgreSQL is willing to give us.
				var max, reserved int
				if err := db.QueryRow("SELECT setting::integer FROM pg_settings WHERE name='max_connections';").Scan(&max); err != nil {
					return nil, fmt.Errorf("failed to find maximum connections: %w", err)
				}
				if err := db.QueryRow("SELECT setting::integer FROM pg_settings WHERE name='superuser_reserved_connections';").Scan(&reserved); err != nil {
					return nil, fmt.Errorf("failed to find reserved connections: %w", err)
				}
				if configured, allowed := dbProperties.MaxOpenConns(), max-reserved; configured > allowed {
					logrus.Errorf("ERROR: The configured 'max_open_conns' is greater than the %d non-superuser connections that PostgreSQL is configured to allow. This can result in bad performance or deadlocks. Please pay close attention to your configured database connection counts. If you REALLY know what you are doing and want to override this error, pass the --skip-db-sanity option to Dendrite.", allowed)
					return nil, fmt.Errorf("database sanity checks failed")
				}
			}
		}
	}
	return db, nil
}
