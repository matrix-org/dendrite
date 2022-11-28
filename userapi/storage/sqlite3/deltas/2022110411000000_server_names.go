package deltas

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/lib/pq"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"
)

var serverNamesTables = []string{
	"userapi_accounts",
	"userapi_account_datas",
	"userapi_devices",
	"userapi_notifications",
	"userapi_openid_tokens",
	"userapi_profiles",
	"userapi_pushers",
	"userapi_threepids",
}

// These tables have a PRIMARY KEY constraint which we need to drop so
// that we can recreate a new unique index that contains the server name.
var serverNamesDropPK = []string{
	"userapi_accounts",
	"userapi_account_datas",
	"userapi_profiles",
}

// These indices are out of date so let's drop them. They will get recreated
// automatically.
var serverNamesDropIndex = []string{
	"userapi_pusher_localpart_idx",
	"userapi_pusher_app_id_pushkey_localpart_idx",
}

// I know what you're thinking: you're wondering "why doesn't this use $1
// and pass variadic parameters to ExecContext?" — the answer is because
// PostgreSQL doesn't expect the table name to be specified as a substituted
// argument in that way so it results in a syntax error in the query.

func UpServerNames(ctx context.Context, tx *sql.Tx, serverName gomatrixserverlib.ServerName) error {
	for _, table := range serverNamesTables {
		q := fmt.Sprintf(
			"SELECT COUNT(name) FROM sqlite_schema WHERE type='table' AND name=%s;",
			pq.QuoteIdentifier(table),
		)
		var c int
		if err := tx.QueryRowContext(ctx, q).Scan(&c); err != nil || c == 0 {
			continue
		}
		q = fmt.Sprintf(
			"SELECT COUNT(*) FROM pragma_table_info(%s) WHERE name='server_name'",
			pq.QuoteIdentifier(table),
		)
		if err := tx.QueryRowContext(ctx, q).Scan(&c); err != nil || c == 1 {
			logrus.Infof("Table %s already has column, skipping", table)
			continue
		}
		if c == 0 {
			q = fmt.Sprintf(
				"ALTER TABLE %s ADD COLUMN server_name TEXT NOT NULL DEFAULT '';",
				pq.QuoteIdentifier(table),
			)
			if _, err := tx.ExecContext(ctx, q); err != nil {
				return fmt.Errorf("add server name to %q error: %w", table, err)
			}
		}
	}
	for _, table := range serverNamesDropPK {
		q := fmt.Sprintf(
			"SELECT COUNT(name), sql FROM sqlite_schema WHERE type='table' AND name=%s;",
			pq.QuoteIdentifier(table),
		)
		var c int
		var sql string
		if err := tx.QueryRowContext(ctx, q).Scan(&c, &sql); err != nil || c == 0 {
			continue
		}
		q = fmt.Sprintf(`
			%s; -- create temporary table
			INSERT INTO %s SELECT * FROM %s; -- copy data
			DROP TABLE %s; -- drop original table
			ALTER TABLE %s RENAME TO %s; -- rename new table
		`,
			strings.Replace(sql, table, table+"_tmp", 1), // create temporary table
			table+"_tmp", table, // copy data
			table,               // drop original table
			table+"_tmp", table, // rename new table
		)
		if _, err := tx.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("drop PK from %q error: %w", table, err)
		}
	}
	for _, index := range serverNamesDropIndex {
		q := fmt.Sprintf(
			"DROP INDEX IF EXISTS %s;",
			pq.QuoteIdentifier(index),
		)
		if _, err := tx.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("drop index %q error: %w", index, err)
		}
	}
	return nil
}
