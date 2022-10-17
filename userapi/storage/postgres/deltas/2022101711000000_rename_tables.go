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

var renameSequenceMappings = map[string]string{}

func UpRenameTables(ctx context.Context, tx *sql.Tx) error {
	for old, new := range renameTableMappings {
		if _, err := tx.ExecContext(ctx, "ALTER TABLE $1 RENAME TO $2;", old, new); err != nil {
			return fmt.Errorf("rename %q to %q error: %w", old, new, err)
		}
	}
	return nil
}

func DownRenameTables(ctx context.Context, tx *sql.Tx) error {
	for old, new := range renameTableMappings {
		if _, err := tx.ExecContext(ctx, "ALTER TABLE $1 RENAME TO $2;", new, old); err != nil {
			return fmt.Errorf("rename %q to %q error: %w", new, old, err)
		}
	}
	return nil
}
