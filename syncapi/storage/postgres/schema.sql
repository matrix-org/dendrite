CREATE SEQUENCE IF NOT EXISTS syncapi_multiroom_id;

CREATE TABLE IF NOT EXISTS syncapi_multiroom_data (
	id BIGINT PRIMARY KEY DEFAULT nextval('syncapi_multiroom_id'),
	user_id TEXT NOT NULL,
	type TEXT NOT NULL,
	data BYTEA NOT NULL,
	ts TIMESTAMP NOT NULL DEFAULT current_timestamp
);

CREATE UNIQUE INDEX IF NOT EXISTS syncapi_multiroom_data_user_id_type_idx ON syncapi_multiroom_data(user_id, type);

CREATE TABLE IF NOT EXISTS syncapi_multiroom_visibility (
	user_id TEXT NOT NULL,
	type TEXT NOT NULL,
	room_id TEXT NOT NULL,
	expire_ts BIGINT NOT NULL DEFAULT 0,
	PRIMARY KEY(user_id, type, room_id)
)
