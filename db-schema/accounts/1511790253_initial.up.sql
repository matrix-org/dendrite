CREATE TABLE IF NOT EXISTS account_data (
    -- The Matrix user ID localpart for this account
    localpart TEXT NOT NULL,
    -- The room ID for this data (empty string if not specific to a room)
    room_id TEXT,
    -- The account data type
    type TEXT NOT NULL,
    -- The account data content
    content TEXT NOT NULL,

    PRIMARY KEY(localpart, room_id, type)
);

CREATE TABLE IF NOT EXISTS account_accounts (
    -- The Matrix user ID localpart for this account
    localpart TEXT NOT NULL PRIMARY KEY,
    -- When this account was first created, as a unix timestamp (ms resolution).
    created_ts BIGINT NOT NULL,
    -- The password hash for this account. Can be NULL if this is a passwordless account.
    password_hash TEXT
    -- TODO:
    -- is_guest, is_admin, appservice_id, upgraded_ts, devices, any email reset stuff?
);

CREATE TABLE IF NOT EXISTS account_filter (
	-- The filter
	filter TEXT NOT NULL,
	-- The ID
	id SERIAL UNIQUE,
	-- The localpart of the Matrix user ID associated to this filter
	localpart TEXT NOT NULL,

	PRIMARY KEY(id, localpart)
);

CREATE INDEX IF NOT EXISTS account_filter_localpart ON account_filter(localpart);

CREATE TABLE IF NOT EXISTS account_memberships (
    -- The Matrix user ID localpart for the member
    localpart TEXT NOT NULL,
    -- The room this user is a member of
    room_id TEXT NOT NULL,
    -- The ID of the join membership event
    event_id TEXT NOT NULL,

    -- A user can only be member of a room once
    PRIMARY KEY (localpart, room_id)
);

-- Use index to process deletion by ID more efficiently
CREATE UNIQUE INDEX IF NOT EXISTS account_membership_event_id ON account_memberships(event_id);

CREATE TABLE IF NOT EXISTS account_profiles (
    -- The Matrix user ID localpart for this account
    localpart TEXT NOT NULL PRIMARY KEY,
    -- The display name for this account
    display_name TEXT,
    -- The URL of the avatar for this account
    avatar_url TEXT
);

CREATE TABLE IF NOT EXISTS account_threepid (
	-- The third party identifier
	threepid TEXT NOT NULL,
	-- The 3PID medium
	medium TEXT NOT NULL DEFAULT 'email',
	-- The localpart of the Matrix user ID associated to this 3PID
	localpart TEXT NOT NULL,

	PRIMARY KEY(threepid, medium)
);

CREATE INDEX IF NOT EXISTS account_threepid_localpart ON account_threepid(localpart);
