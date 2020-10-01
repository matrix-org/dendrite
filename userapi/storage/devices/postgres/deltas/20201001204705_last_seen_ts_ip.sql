-- +goose Up
-- +goose StatementBegin
ALTER TABLE device_devices ADD COLUMN last_seen_ts BIGINT NOT NULL;
ALTER TABLE device_devices ADD COLUMN ip TEXT;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE device_devices DROP COLUMN last_seen_ts;
ALTER TABLE device_devices DROP COLUMN ip;
-- +goose StatementEnd
