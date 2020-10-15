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
ALTER TABLE device_devices ADD COLUMN IF NOT EXISTS last_seen_ts BIGINT NOT NULL DEFAULT EXTRACT(EPOCH FROM CURRENT_TIMESTAMP)*1000;
ALTER TABLE device_devices ADD COLUMN IF NOT EXISTS ip TEXT;
ALTER TABLE device_devices ADD COLUMN IF NOT EXISTS user_agent TEXT;`)
	if err != nil {
		return fmt.Errorf("failed to execute upgrade: %w", err)
	}
	return nil
}

func DownLastSeenTSIP(tx *sql.Tx) error {
	_, err := tx.Exec(`
	ALTER TABLE device_devices DROP COLUMN last_seen_ts;
	ALTER TABLE device_devices DROP COLUMN ip;
	ALTER TABLE device_devices DROP COLUMN user_agent;`)
	if err != nil {
		return fmt.Errorf("failed to execute downgrade: %w", err)
	}
	return nil
}
