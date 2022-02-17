package deltas

import (
	"database/sql"
	"fmt"

	"github.com/pressly/goose"

	"github.com/matrix-org/dendrite/internal/sqlutil"
)

func LoadFromGoose() {
	goose.AddMigration(UpIsActive, DownIsActive)
	goose.AddMigration(UpAddAccountType, DownAddAccountType)
}

func LoadIsActive(m *sqlutil.Migrations) {
	m.AddMigration(UpIsActive, DownIsActive)
}

func UpIsActive(tx *sql.Tx) error {
	_, err := tx.Exec("ALTER TABLE account_accounts ADD COLUMN IF NOT EXISTS is_deactivated BOOLEAN DEFAULT FALSE;")
	if err != nil {
		return fmt.Errorf("failed to execute upgrade: %w", err)
	}
	return nil
}

func DownIsActive(tx *sql.Tx) error {
	_, err := tx.Exec("ALTER TABLE account_accounts DROP COLUMN is_deactivated;")
	if err != nil {
		return fmt.Errorf("failed to execute downgrade: %w", err)
	}
	return nil
}
