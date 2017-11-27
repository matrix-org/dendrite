-- Stores data about devices.
CREATE TABLE IF NOT EXISTS device_devices (
    -- The access token granted to this device. This has to be the primary key
    -- so we can distinguish which device is making a given request.
    access_token TEXT NOT NULL PRIMARY KEY,
    -- The device identifier. This only needs to uniquely identify a device for a given user, not globally.
    -- access_tokens will be clobbered based on the device ID for a user.
    device_id TEXT NOT NULL,
    -- The Matrix user ID localpart for this device. This is preferable to storing the full user_id
    -- as it is smaller, makes it clearer that we only manage devices for our own users, and may make
    -- migration to different domain names easier.
    localpart TEXT NOT NULL,
    -- When this devices was first recognised on the network, as a unix timestamp (ms resolution).
    created_ts BIGINT NOT NULL,
    -- The display name, human friendlier than device_id and updatable
    display_name TEXT
    -- TODO: device keys, device display names, last used ts and IP address?, token restrictions (if 3rd-party OAuth app)
);

-- Device IDs must be unique for a given user.
CREATE UNIQUE INDEX IF NOT EXISTS device_localpart_id_idx ON device_devices(localpart, device_id);
