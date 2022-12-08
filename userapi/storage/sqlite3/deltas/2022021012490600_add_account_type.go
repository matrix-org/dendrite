package deltas

import (
	"context"
	"database/sql"
	"fmt"
)

func UpAddAccountType(ctx context.Context, tx *sql.Tx) error {
	// initially set every account to useraccount, change appservice and guest accounts afterwards
	// (user = 1, guest = 2, admin = 3, appservice = 4)
	_, err := tx.ExecContext(ctx, `ALTER TABLE userapi_accounts RENAME TO userapi_accounts_tmp;
CREATE TABLE userapi_accounts (
    localpart TEXT NOT NULL PRIMARY KEY,
	server_name TEXT NOT NULL,
    created_ts BIGINT NOT NULL,
    password_hash TEXT,
    appservice_id TEXT,
    is_deactivated BOOLEAN DEFAULT 0,
    account_type INTEGER NOT NULL
);
INSERT
    INTO userapi_accounts (
      localpart, created_ts, password_hash, appservice_id, account_type
    ) SELECT
        localpart, created_ts, password_hash, appservice_id, 1
    FROM userapi_accounts_tmp
;
UPDATE userapi_accounts SET account_type = 4 WHERE appservice_id <> '';
UPDATE userapi_accounts SET account_type = 2 WHERE localpart GLOB '[0-9]*';
DROP TABLE userapi_accounts_tmp;`)
	if err != nil {
		return fmt.Errorf("failed to add column: %w", err)
	}
	return nil
}

func DownAddAccountType(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `ALTER TABLE userapi_accounts DROP COLUMN account_type;`)
	if err != nil {
		return fmt.Errorf("failed to execute downgrade: %w", err)
	}
	return nil
}
