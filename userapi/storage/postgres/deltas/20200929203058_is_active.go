package deltas

import (
	"context"
	"database/sql"
	"fmt"
)

func UpIsActive(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, "ALTER TABLE userapi_accounts ADD COLUMN IF NOT EXISTS is_deactivated BOOLEAN DEFAULT FALSE;")
	if err != nil {
		return fmt.Errorf("failed to execute upgrade: %w", err)
	}
	return nil
}

func DownIsActive(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, "ALTER TABLE userapi_accounts DROP COLUMN is_deactivated;")
	if err != nil {
		return fmt.Errorf("failed to execute downgrade: %w", err)
	}
	return nil
}
