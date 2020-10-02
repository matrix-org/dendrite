-- +goose Up
-- +goose StatementBegin
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
DROP TABLE account_accounts_tmp;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
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
DROP TABLE account_accounts_tmp;
-- +goose StatementEnd
