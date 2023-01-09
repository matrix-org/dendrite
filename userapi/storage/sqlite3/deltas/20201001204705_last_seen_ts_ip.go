package deltas

import (
	"context"
	"database/sql"
	"fmt"
)

func UpLastSeenTSIP(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `
    ALTER TABLE userapi_devices RENAME TO userapi_devices_tmp;
    CREATE TABLE userapi_devices (
        access_token TEXT PRIMARY KEY,
        session_id INTEGER,
        device_id TEXT ,
        localpart TEXT ,
		server_name TEXT NOT NULL,
        created_ts BIGINT,
        display_name TEXT,
        last_seen_ts BIGINT,
        ip TEXT,
        user_agent TEXT,
        UNIQUE (localpart, device_id)
    );
    INSERT
    INTO userapi_devices (
        access_token, session_id, device_id, localpart, created_ts, display_name, last_seen_ts, ip, user_agent
    )  SELECT
           access_token, session_id, device_id, localpart, created_ts, display_name, created_ts, '', ''
    FROM userapi_devices_tmp;
    DROP TABLE userapi_devices_tmp;`)
	if err != nil {
		return fmt.Errorf("failed to execute upgrade: %w", err)
	}
	return nil
}

func DownLastSeenTSIP(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `
ALTER TABLE userapi_devices RENAME TO userapi_devices_tmp;
CREATE TABLE IF NOT EXISTS userapi_devices (
    access_token TEXT PRIMARY KEY,
    session_id INTEGER,
    device_id TEXT ,
    localpart TEXT ,
    created_ts BIGINT,
    display_name TEXT,
    UNIQUE (localpart, device_id)
);
INSERT
INTO userapi_devices (
    access_token, session_id, device_id, localpart, created_ts, display_name
) SELECT
       access_token, session_id, device_id, localpart, created_ts, display_name
FROM userapi_devices_tmp;
DROP TABLE userapi_devices_tmp;`)
	if err != nil {
		return fmt.Errorf("failed to execute downgrade: %w", err)
	}
	return nil
}
