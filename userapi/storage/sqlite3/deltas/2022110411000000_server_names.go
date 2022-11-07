package deltas

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/lib/pq"
	"github.com/matrix-org/gomatrixserverlib"
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

// I know what you're thinking: you're wondering "why doesn't this use $1
// and pass variadic parameters to ExecContext?" — the answer is because
// PostgreSQL doesn't expect the table name to be specified as a substituted
// argument in that way so it results in a syntax error in the query.

func UpServerNames(ctx context.Context, tx *sql.Tx, serverName gomatrixserverlib.ServerName) error {
	for _, table := range serverNamesTables {
		q := fmt.Sprintf(
			"SELECT name FROM sqlite_schema WHERE type='table' AND name=%s;",
			pq.QuoteIdentifier(table),
		)
		var c int
		if err := tx.QueryRowContext(ctx, q).Scan(&c); err != nil || c == 0 {
			continue
		}
		q = fmt.Sprintf(
			"ALTER TABLE %s ADD COLUMN IF NOT EXISTS server_name TEXT NOT NULL DEFAULT '';",
			pq.QuoteIdentifier(table),
		)
		if _, err := tx.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("add server name to %q error: %w", table, err)
		}
		q = fmt.Sprintf(
			"UPDATE %s SET server_name = %s WHERE server_name = '';",
			pq.QuoteIdentifier(table), pq.QuoteLiteral(string(serverName)),
		)
		if _, err := tx.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("write server names to %q error: %w", table, err)
		}
	}
	return nil
}

func DownServerNames(ctx context.Context, tx *sql.Tx) error {
	for _, table := range serverNamesTables {
		q := fmt.Sprintf(
			"ALTER TABLE %s DELETE COLUMN server_name;",
			pq.QuoteIdentifier(table),
		)
		if _, err := tx.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("remove server name from %q error: %w", table, err)
		}
	}
	return nil
}
