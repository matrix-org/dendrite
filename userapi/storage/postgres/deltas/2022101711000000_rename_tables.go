package deltas

import (
	"context"
	"database/sql"
	"fmt"
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

func UpRenameTables(ctx context.Context, tx *sql.Tx) error {
	for old, new := range renameTableMappings {
		if _, err := tx.ExecContext(ctx, "ALTER TABLE IF EXISTS $1 RENAME TO $2;", old, new); err != nil {
			return fmt.Errorf("rename %q to %q error: %w", old, new, err)
		}
	}
	for old, new := range renameSequenceMappings {
		if _, err := tx.ExecContext(ctx, "ALTER SEQUENCE IF EXISTS $1 RENAME TO $2;", old, new); err != nil {
			return fmt.Errorf("rename %q to %q error: %w", old, new, err)
		}
	}
	for old, new := range renameIndicesMappings {
		if _, err := tx.ExecContext(ctx, "ALTER INDEX IF EXISTS $1 RENAME TO $2;", old, new); err != nil {
			return fmt.Errorf("rename %q to %q error: %w", old, new, err)
		}
	}
	return nil
}

func DownRenameTables(ctx context.Context, tx *sql.Tx) error {
	for old, new := range renameTableMappings {
		if _, err := tx.ExecContext(ctx, "ALTER TABLE IF EXISTS $1 RENAME TO $2;", new, old); err != nil {
			return fmt.Errorf("rename %q to %q error: %w", new, old, err)
		}
	}
	for old, new := range renameSequenceMappings {
		if _, err := tx.ExecContext(ctx, "ALTER SEQUENCE IF EXISTS $1 RENAME TO $2;", new, old); err != nil {
			return fmt.Errorf("rename %q to %q error: %w", new, old, err)
		}
	}
	for old, new := range renameIndicesMappings {
		if _, err := tx.ExecContext(ctx, "ALTER INDEX IF EXISTS $1 RENAME TO $2;", new, old); err != nil {
			return fmt.Errorf("rename %q to %q error: %w", new, old, err)
		}
	}
	return nil
}
