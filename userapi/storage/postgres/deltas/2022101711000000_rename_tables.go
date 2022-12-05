package deltas

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/lib/pq"
)

var renameTableMappings = map[string]string{
	"account_accounts":               "userapi_accounts",
	"account_data":                   "userapi_account_datas",
	"device_devices":                 "userapi_devices",
	"account_e2e_room_keys":          "userapi_key_backups",
	"account_e2e_room_keys_versions": "userapi_key_backup_versions",
	"login_tokens":                   "userapi_login_tokens",
	"open_id_tokens":                 "userapi_openid_tokens",
	"account_profiles":               "userapi_profiles",
	"account_threepid":               "userapi_threepids",
}

var renameSequenceMappings = map[string]string{
	"device_session_id_seq":              "userapi_device_session_id_seq",
	"account_e2e_room_keys_versions_seq": "userapi_key_backup_versions_seq",
}

var renameIndicesMappings = map[string]string{
	"device_localpart_id_idx":            "userapi_device_localpart_id_idx",
	"e2e_room_keys_idx":                  "userapi_key_backups_idx",
	"e2e_room_keys_versions_idx":         "userapi_key_backups_versions_idx",
	"account_e2e_room_keys_versions_idx": "userapi_key_backup_versions_idx",
	"login_tokens_expiration_idx":        "userapi_login_tokens_expiration_idx",
	"account_threepid_localpart":         "userapi_threepid_idx",
}

// I know what you're thinking: you're wondering "why doesn't this use $1
// and pass variadic parameters to ExecContext?" — the answer is because
// PostgreSQL doesn't expect the table name to be specified as a substituted
// argument in that way so it results in a syntax error in the query.

func UpRenameTables(ctx context.Context, tx *sql.Tx) error {
	for old, new := range renameTableMappings {
		q := fmt.Sprintf(
			"ALTER TABLE IF EXISTS %s RENAME TO %s;",
			pq.QuoteIdentifier(old), pq.QuoteIdentifier(new),
		)
		if _, err := tx.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("rename table %q to %q error: %w", old, new, err)
		}
	}
	for old, new := range renameSequenceMappings {
		q := fmt.Sprintf(
			"ALTER SEQUENCE IF EXISTS %s RENAME TO %s;",
			pq.QuoteIdentifier(old), pq.QuoteIdentifier(new),
		)
		if _, err := tx.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("rename table %q to %q error: %w", old, new, err)
		}
	}
	for old, new := range renameIndicesMappings {
		q := fmt.Sprintf(
			"ALTER INDEX IF EXISTS %s RENAME TO %s;",
			pq.QuoteIdentifier(old), pq.QuoteIdentifier(new),
		)
		if _, err := tx.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("rename table %q to %q error: %w", old, new, err)
		}
	}
	return nil
}

func DownRenameTables(ctx context.Context, tx *sql.Tx) error {
	for old, new := range renameTableMappings {
		q := fmt.Sprintf(
			"ALTER TABLE IF EXISTS %s RENAME TO %s;",
			pq.QuoteIdentifier(new), pq.QuoteIdentifier(old),
		)
		if _, err := tx.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("rename table %q to %q error: %w", new, old, err)
		}
	}
	for old, new := range renameSequenceMappings {
		q := fmt.Sprintf(
			"ALTER SEQUENCE IF EXISTS %s RENAME TO %s;",
			pq.QuoteIdentifier(new), pq.QuoteIdentifier(old),
		)
		if _, err := tx.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("rename table %q to %q error: %w", new, old, err)
		}
	}
	for old, new := range renameIndicesMappings {
		q := fmt.Sprintf(
			"ALTER INDEX IF EXISTS %s RENAME TO %s;",
			pq.QuoteIdentifier(new), pq.QuoteIdentifier(old),
		)
		if _, err := tx.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("rename table %q to %q error: %w", new, old, err)
		}
	}
	return nil
}
