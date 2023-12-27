package deltas

import (
	"context"
	"database/sql"
	"fmt"
)

func UpUniquePusher(ctx context.Context, tx *sql.Tx) error {
	rows := tx.QueryRowContext(ctx, "SELECT EXISTS (select * from pg_tables where tablename = 'userapi_pushers')")
	tableExists := false
	err := rows.Scan(&tableExists)

	if err != nil {
		return fmt.Errorf("select table exists: %w", err)
	}
	if !tableExists {
		return nil
	}
	_, err = tx.ExecContext(ctx, "DELETE FROM userapi_pushers p1 USING userapi_pushers p2 WHERE p1.pushkey_ts_ms < p2.pushkey_ts_ms AND p1.app_id = p2.app_id AND p1.pushkey = p2.pushkey")
	if err != nil {
		return fmt.Errorf("delete pusher duplicates: %w", err)
	}
	_, err = tx.ExecContext(ctx, "DROP INDEX IF EXISTS userapi_pusher_app_id_pushkey_localpart_idx")
	if err != nil {
		return fmt.Errorf("drop unique index: %w", err)
	}
	_, err = tx.ExecContext(ctx, "DROP INDEX IF EXISTS userapi_pusher_app_id_pushkey_idx")
	if err != nil {
		return fmt.Errorf("drop index: %w", err)
	}
	return nil
}

func DownUniquePusher(ctx context.Context, tx *sql.Tx) error {
	return nil
}
