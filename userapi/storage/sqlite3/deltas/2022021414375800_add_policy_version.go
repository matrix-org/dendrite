package deltas

import (
	"database/sql"
	"fmt"

	"github.com/matrix-org/dendrite/internal/sqlutil"
)

func LoadAddPolicyVersion(m *sqlutil.Migrations) {
	m.AddMigration(UpAddPolicyVersion, DownAddPolicyVersion)
}

func UpAddPolicyVersion(tx *sql.Tx) error {
	_, err := tx.Exec("ALTER TABLE account_accounts ADD COLUMN policy_version TEXT;")
	if err != nil {
		return fmt.Errorf("failed to execute upgrade: %w", err)
	}
	_, err = tx.Exec("ALTER TABLE account_accounts ADD COLUMN policy_version_sent TEXT;")
	if err != nil {
		return fmt.Errorf("failed to execute upgrade: %w", err)
	}
	_, err = tx.Exec("ALTER TABLE account_accounts ADD COLUMN server_notice_room_id TEXT;")
	if err != nil {
		return fmt.Errorf("failed to execute upgrade: %w", err)
	}
	return nil
}

func DownAddPolicyVersion(tx *sql.Tx) error {
	_, err := tx.Exec("ALTER TABLE account_accounts DROP COLUMN policy_version;")
	if err != nil {
		return fmt.Errorf("failed to execute downgrade: %w", err)
	}
	_, err = tx.Exec("ALTER TABLE account_accounts DROP COLUMN policy_version_sent;")
	if err != nil {
		return fmt.Errorf("failed to execute downgrade: %w", err)
	}
	_, err = tx.Exec("ALTER TABLE account_accounts DROP COLUMN server_notice_room_id;")
	if err != nil {
		return fmt.Errorf("failed to execute downgrade: %w", err)
	}
	return nil
}
