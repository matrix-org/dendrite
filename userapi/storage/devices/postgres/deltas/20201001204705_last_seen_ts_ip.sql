-- +goose Up
-- +goose StatementBegin
ALTER TABLE device_devices ADD COLUMN IF NOT EXISTS last_seen_ts BIGINT NOT NULL;
ALTER TABLE device_devices ADD COLUMN IF NOT EXISTS ip TEXT;
ALTER TABLE device_devices ADD COLUMN IF NOT EXISTS user_agent TEXT;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE device_devices DROP COLUMN last_seen_ts;
ALTER TABLE device_devices DROP COLUMN ip;
ALTER TABLE device_devices DROP COLUMN user_agent;
-- +goose StatementEnd
