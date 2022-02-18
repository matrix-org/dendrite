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
	// initially set every account to useraccount, change appservice and guest accounts afterwards
	// (user = 1, guest = 2, admin = 3, appservice = 4)
	_, err := tx.Exec(`ALTER TABLE account_accounts RENAME TO account_accounts_tmp;
CREATE TABLE account_accounts (
    localpart TEXT NOT NULL PRIMARY KEY,
    created_ts BIGINT NOT NULL,
    password_hash TEXT,
    appservice_id TEXT,
    is_deactivated BOOLEAN DEFAULT 0,
    account_type INTEGER NOT NULL
);
INSERT
    INTO account_accounts (
      localpart, created_ts, password_hash, appservice_id, account_type
    ) SELECT
        localpart, created_ts, password_hash, appservice_id, 1
    FROM account_accounts_tmp
;
UPDATE account_accounts SET account_type = 4 WHERE appservice_id <> '';
UPDATE account_accounts SET account_type = 2 WHERE localpart GLOB '[0-9]*';
DROP TABLE account_accounts_tmp;`)
	if err != nil {
		return fmt.Errorf("failed to add column: %w", err)
	}
	return nil
}

func DownAddAccountType(tx *sql.Tx) error {
	_, err := tx.Exec(`ALTER TABLE account_accounts DROP COLUMN account_type;`)
	if err != nil {
		return fmt.Errorf("failed to execute downgrade: %w", err)
	}
	return nil
}
