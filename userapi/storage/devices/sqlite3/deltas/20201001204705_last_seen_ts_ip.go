package deltas

import (
	"database/sql"
	"fmt"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/pressly/goose"
)

func LoadFromGoose() {
	goose.AddMigration(UpLastSeenTSIP, DownLastSeenTSIP)
}

func LoadLastSeenTSIP(m *sqlutil.Migrations) {
	m.AddMigration(UpLastSeenTSIP, DownLastSeenTSIP)
}

func UpLastSeenTSIP(tx *sql.Tx) error {
	_, err := tx.Exec(`
    ALTER TABLE device_devices RENAME TO device_devices_tmp;
    CREATE TABLE device_devices (
        access_token TEXT PRIMARY KEY,
        session_id INTEGER,
        device_id TEXT ,
        localpart TEXT ,
        created_ts BIGINT,
        display_name TEXT,
        last_seen_ts BIGINT,
        ip TEXT,
        user_agent TEXT,
        UNIQUE (localpart, device_id)
    );
    INSERT
    INTO device_devices (
        access_token, session_id, device_id, localpart, created_ts, display_name, last_seen_ts, ip, user_agent
    )  SELECT
           access_token, session_id, device_id, localpart, created_ts, display_name, created_ts, '', ''
    FROM device_devices_tmp;
    DROP TABLE device_devices_tmp;`)
	if err != nil {
		return fmt.Errorf("failed to execute upgrade: %w", err)
	}
	return nil
}

func DownLastSeenTSIP(tx *sql.Tx) error {
	_, err := tx.Exec(`
ALTER TABLE device_devices RENAME TO device_devices_tmp;
CREATE TABLE IF NOT EXISTS device_devices (
    access_token TEXT PRIMARY KEY,
    session_id INTEGER,
    device_id TEXT ,
    localpart TEXT ,
    created_ts BIGINT,
    display_name TEXT,
    UNIQUE (localpart, device_id)
);
INSERT
INTO device_devices (
    access_token, session_id, device_id, localpart, created_ts, display_name
) SELECT
       access_token, session_id, device_id, localpart, created_ts, display_name
FROM device_devices_tmp;
DROP TABLE device_devices_tmp;`)
	if err != nil {
		return fmt.Errorf("failed to execute downgrade: %w", err)
	}
	return nil
}
