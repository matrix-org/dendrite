package deltas

import (
	"database/sql"
	"fmt"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/pressly/goose"
)

func LoadFromGoose() {
	goose.AddMigration(UpIsActive, DownIsActive)
}

func LoadIsActive(m *sqlutil.Migrations) {
	m.AddMigration(UpIsActive, DownIsActive)
}

func UpIsActive(tx *sql.Tx) error {
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
		return fmt.Errorf("failed to execute upgrade: %w", err)
	}
	return nil
}

func DownIsActive(tx *sql.Tx) error {
	_, err := tx.Exec(`
	ALTER TABLE account_accounts RENAME TO account_accounts_tmp;
CREATE TABLE account_accounts (
    localpart TEXT NOT NULL PRIMARY KEY,
    created_ts BIGINT NOT NULL,
    password_hash TEXT,
    appservice_id TEXT
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
