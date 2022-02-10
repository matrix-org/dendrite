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
	_, err := tx.Exec(`
	ALTER TABLE account_accounts RENAME TO account_accounts_tmp;
CREATE TABLE account_accounts (
    localpart TEXT NOT NULL PRIMARY KEY,
    created_ts BIGINT NOT NULL,
    password_hash TEXT,
    appservice_id TEXT,
    is_deactivated BOOLEAN DEFAULT 0,
	account_type INTEGER DEFAULT 2
);
INSERT INTO account_accounts (
      localpart, created_ts, password_hash, appservice_id 
    ) SELECT
        localpart, created_ts, password_hash, appservice_id
    FROM account_accounts_tmp;

UPDATE account_accounts SET account_type = 4 WHERE appservice_id <> '';

DROP TABLE account_accounts_tmp;`)
	if err != nil {
		return fmt.Errorf("failed to execute upgrade: %w", err)
	}
	return nil
}

func DownAddAccountType(tx *sql.Tx) error {
	_, err := tx.Exec(`
	ALTER TABLE account_accounts RENAME TO account_accounts_tmp;
CREATE TABLE account_accounts (
    localpart TEXT NOT NULL PRIMARY KEY,
    created_ts BIGINT NOT NULL,
    password_hash TEXT,
    appservice_id TEXT,
    is_deactivated BOOLEAN DEFAULT 0
);
INSERT
    INTO account_accounts (
      localpart, created_ts, password_hash, appservice_id 
    ) SELECT
        localpart, created_ts, password_hash, appservice_id
    FROM account_accounts_tmp
;
DROP TABLE account_accounts_tmp;`)
	if err != nil {
		return fmt.Errorf("failed to execute downgrade: %w", err)
	}
	return nil
}
