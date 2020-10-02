-- +goose Up
-- +goose StatementBegin
ALTER TABLE account_accounts ADD COLUMN IF NOT EXISTS is_deactivated BOOLEAN DEFAULT FALSE;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE account_accounts DROP COLUMN is_deactivated;
-- +goose StatementEnd
