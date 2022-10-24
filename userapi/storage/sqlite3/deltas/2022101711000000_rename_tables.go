package deltas

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
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

var renameIndicesMappings = map[string]string{
	"device_localpart_id_idx":            "userapi_device_localpart_id_idx",
	"e2e_room_keys_idx":                  "userapi_key_backups_idx",
	"e2e_room_keys_versions_idx":         "userapi_key_backups_versions_idx",
	"account_e2e_room_keys_versions_idx": "userapi_key_backup_versions_idx",
	"login_tokens_expiration_idx":        "userapi_login_tokens_expiration_idx",
	"account_threepid_localpart":         "userapi_threepid_idx",
}

func UpRenameTables(ctx context.Context, tx *sql.Tx) error {
	for old, new := range renameTableMappings {
		// SQLite has no "IF EXISTS" so check if the table exists.
		var name string
		if err := tx.QueryRowContext(
			ctx, "SELECT name FROM sqlite_schema WHERE type = 'table' AND name = $1;", old,
		).Scan(&name); err != nil {
			if err == sql.ErrNoRows {
				continue
			}
			return err
		}
		q := fmt.Sprintf(
			"ALTER TABLE %s RENAME TO %s;", old, new,
		)
		if _, err := tx.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("rename table %q to %q error: %w", old, new, err)
		}
	}
	for old, new := range renameIndicesMappings {
		var query string
		if err := tx.QueryRowContext(
			ctx, "SELECT sql FROM sqlite_schema WHERE type = 'index' AND name = $1;", old,
		).Scan(&query); err != nil {
			if err == sql.ErrNoRows {
				continue
			}
			return err
		}
		query = strings.Replace(query, old, new, 1)
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("DROP INDEX %s;", old)); err != nil {
			return fmt.Errorf("drop index %q to %q error: %w", old, new, err)
		}
		if _, err := tx.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("recreate index %q to %q error: %w", old, new, err)
		}
	}
	return nil
}

func DownRenameTables(ctx context.Context, tx *sql.Tx) error {
	for old, new := range renameTableMappings {
		// SQLite has no "IF EXISTS" so check if the table exists.
		var name string
		if err := tx.QueryRowContext(
			ctx, "SELECT name FROM sqlite_schema WHERE type = 'table' AND name = $1;", new,
		).Scan(&name); err != nil {
			if err == sql.ErrNoRows {
				continue
			}
			return err
		}
		q := fmt.Sprintf(
			"ALTER TABLE %s RENAME TO %s;", new, old,
		)
		if _, err := tx.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("rename table %q to %q error: %w", new, old, err)
		}
	}
	for old, new := range renameIndicesMappings {
		var query string
		if err := tx.QueryRowContext(
			ctx, "SELECT sql FROM sqlite_schema WHERE type = 'index' AND name = $1;", new,
		).Scan(&query); err != nil {
			if err == sql.ErrNoRows {
				continue
			}
			return err
		}
		query = strings.Replace(query, new, old, 1)
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("DROP INDEX %s;", new)); err != nil {
			return fmt.Errorf("drop index %q to %q error: %w", new, old, err)
		}
		if _, err := tx.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("recreate index %q to %q error: %w", new, old, err)
		}
	}
	return nil
}
