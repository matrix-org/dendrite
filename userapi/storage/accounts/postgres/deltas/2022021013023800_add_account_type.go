package deltas

import (
	"database/sql"
	"fmt"

	"github.com/matrix-org/dendrite/internal/sqlutil"
)

func LoadAddAccountType(m *sqlutil.Migrations) {
	m.AddMigration(UpAddAccountType, DownAddAccountType)
}

func UpAddAccountType(tx *sql.Tx) error {
	_, err := tx.Exec("ALTER TABLE account_accounts ADD COLUMN IF NOT EXISTS account_type SMALLINT;")
	if err != nil {
		return fmt.Errorf("failed to add column: %w", err)

	}
	_, err = tx.Exec("UPDATE account_accounts SET account_type = 1 WHERE appservice_id = '';")
	if err != nil {
		return fmt.Errorf("failed to update user accounts: %w", err)
	}
	_, err = tx.Exec("UPDATE account_accounts SET account_type = 4 WHERE appservice_id <> '';")
	if err != nil {
		return fmt.Errorf("failed to update appservice accounts upgrade: %w", err)
	}
	return nil
}

func DownAddAccountType(tx *sql.Tx) error {
	_, err := tx.Exec("ALTER TABLE account_accounts DROP COLUMN account_type;")
	if err != nil {
		return fmt.Errorf("failed to execute downgrade: %w", err)
	}
	return nil
}
