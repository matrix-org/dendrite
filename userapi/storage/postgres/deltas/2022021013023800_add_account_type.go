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
	// initially set every account to useraccount, change appservice and guest accounts afterwards
	// (user = 1, guest = 2, admin = 3, appservice = 4)
	_, err := tx.Exec(`ALTER TABLE account_accounts ADD COLUMN IF NOT EXISTS account_type SMALLINT NOT NULL DEFAULT 1;
UPDATE account_accounts SET account_type = 4 WHERE appservice_id <> '';
UPDATE account_accounts SET account_type = 2 WHERE localpart ~ '^[0-9]+$';
ALTER TABLE account_accounts ALTER COLUMN account_type DROP DEFAULT;`,
	)
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
