-- +goose Up
-- +goose StatementBegin
ALTER TABLE device_devices ADD COLUMN last_used_ts BIGINT NOT NULL;
ALTER TABLE device_devices ADD COLUMN ip TEXT;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE device_devices DROP COLUMN last_used_ts;
ALTER TABLE device_devices DROP COLUMN ip;
-- +goose StatementEnd
