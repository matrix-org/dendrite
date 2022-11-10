package deltas

import (
	"context"
	"database/sql"
	"fmt"
)

func UpLastSeenTSIP(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `
ALTER TABLE userapi_devices ADD COLUMN IF NOT EXISTS last_seen_ts BIGINT NOT NULL DEFAULT EXTRACT(EPOCH FROM CURRENT_TIMESTAMP)*1000;
ALTER TABLE userapi_devices ADD COLUMN IF NOT EXISTS ip TEXT;
ALTER TABLE userapi_devices ADD COLUMN IF NOT EXISTS user_agent TEXT;`)
	if err != nil {
		return fmt.Errorf("failed to execute upgrade: %w", err)
	}
	return nil
}

func DownLastSeenTSIP(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `
	ALTER TABLE userapi_devices DROP COLUMN last_seen_ts;
	ALTER TABLE userapi_devices DROP COLUMN ip;
	ALTER TABLE userapi_devices DROP COLUMN user_agent;`)
	if err != nil {
		return fmt.Errorf("failed to execute downgrade: %w", err)
	}
	return nil
}
