package deltas

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/matrix-org/gomatrixserverlib"
)

func UpProfilePrimaryKey(serverName gomatrixserverlib.ServerName) func(context.Context, *sql.Tx) error {
	return func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, fmt.Sprintf(`ALTER TABLE account_profiles ADD COLUMN IF NOT EXISTS server_name TEXT NOT NULL DEFAULT '%s';
		ALTER TABLE account_profiles DROP CONSTRAINT account_profiles_pkey;
		ALTER TABLE account_profiles ADD PRIMARY KEY (localpart, server_name);
		ALTER TABLE account_profiles ALTER COLUMN server_name DROP DEFAULT;`, serverName))
		if err != nil {
			return fmt.Errorf("failed to execute upgrade: %w", err)
		}
		return nil
	}
}

func DownProfilePrimaryKey(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `ALTER TABLE account_profiles DROP COLUMN IF EXISTS server_name;
		ALTER TABLE account_profiles ADD PRIMARY KEY(localpart);`)
	if err != nil {
		return fmt.Errorf("failed to execute downgrade: %w", err)
	}
	return nil
}
