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

// These tables have a PRIMARY KEY constraint which we need to drop so
// that we can recreate a new unique index that contains the server name.
// If the new key doesn't exist (i.e. the database was created before the
// table rename migration) we'll try to drop the old one instead.
var serverNamesDropPK = map[string]string{
	"userapi_accounts":      "account_accounts",
	"userapi_account_datas": "account_data",
	"userapi_profiles":      "account_profiles",
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
			"ALTER TABLE IF EXISTS %s ADD COLUMN IF NOT EXISTS server_name TEXT NOT NULL DEFAULT '';",
			pq.QuoteIdentifier(table),
		)
		if _, err := tx.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("add server name to %q error: %w", table, err)
		}
	}
	for newTable, oldTable := range serverNamesDropPK {
		q := fmt.Sprintf(
			"ALTER TABLE IF EXISTS %s DROP CONSTRAINT IF EXISTS %s;",
			pq.QuoteIdentifier(newTable), pq.QuoteIdentifier(newTable+"_pkey"),
		)
		if _, err := tx.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("drop new PK from %q error: %w", newTable, err)
		}
		q = fmt.Sprintf(
			"ALTER TABLE IF EXISTS %s DROP CONSTRAINT IF EXISTS %s;",
			pq.QuoteIdentifier(newTable), pq.QuoteIdentifier(oldTable+"_pkey"),
		)
		if _, err := tx.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("drop old PK from %q error: %w", newTable, err)
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
