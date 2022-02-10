package deltas

import (
	"database/sql"
	"fmt"

	"github.com/pressly/goose"

	"github.com/matrix-org/dendrite/internal/sqlutil"
)

func init() {
	goose.AddMigration(UpAddAccountType, DownAddAccountType)
}

func LoadAddAccountType(m *sqlutil.Migrations) {
	m.AddMigration(UpAddAccountType, DownAddAccountType)
}

func UpAddAccountType(tx *sql.Tx) error {
	_, err := tx.Exec("ALTER TABLE account_accounts ADD COLUMN IF NOT EXISTS account_type INT DEFAULT 2;")
	if err != nil {
		return fmt.Errorf("failed to execute upgrade: %w", err)
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
