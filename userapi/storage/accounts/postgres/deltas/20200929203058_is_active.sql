-- +goose Up
-- +goose StatementBegin
ALTER TABLE account_accounts ADD COLUMN IF NOT EXISTS is_active BOOLEAN DEFAULT TRUE;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE account_accounts DROP COLUMN is_active;
-- +goose StatementEnd
