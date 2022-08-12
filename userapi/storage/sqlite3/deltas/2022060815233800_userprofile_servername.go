package deltas

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/matrix-org/gomatrixserverlib"
)

func UpProfilePrimaryKey(serverName gomatrixserverlib.ServerName) func(context.Context, *sql.Tx) error {
	return func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, fmt.Sprintf(`
    ALTER TABLE account_profiles RENAME TO account_profiles_tmp;
	CREATE TABLE IF NOT EXISTS account_profiles (
    	localpart TEXT NOT NULL,
    	server_name TEXT NOT NULL,
    	display_name TEXT,
    	avatar_url TEXT,
    	PRIMARY KEY (localpart, server_name)
	);
    INSERT
    INTO account_profiles (
        localpart, server_name, display_name, avatar_url
    )  SELECT
           localpart, '%s', display_name, avatar_url
    FROM account_profiles_tmp;
    DROP TABLE account_profiles_tmp;`, serverName))
		if err != nil {
			return fmt.Errorf("failed to execute upgrade: %w", err)
		}
		return nil
	}
}

func DownProfilePrimaryKey(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `
    ALTER TABLE account_profiles RENAME TO account_profiles_tmp;
	CREATE TABLE IF NOT EXISTS account_profiles (
    	localpart TEXT NOT NULL PRIMARY KEY,
    	display_name TEXT,
    	avatar_url TEXT
	);
    INSERT
    INTO account_profiles (
        localpart, display_name, avatar_url
    )  SELECT
           localpart, display_name, avatar_url
    FROM account_profiles_tmp;
    DROP TABLE account_profiles_tmp;`)
	if err != nil {
		return fmt.Errorf("failed to execute downgrade: %w", err)
	}
	return nil
}
