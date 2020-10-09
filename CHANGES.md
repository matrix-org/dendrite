# Dendrite 0.1.0 (2020-10-08)

First versioned release of Dendrite.

## Client-Server API Features

### Account registration and management
- Registration: By password only.
- Login: By password only. No fallback.
- Logout: Yes.
- Change password: Yes.
- Link email/msisdn to account: No.
- Deactivate account: Yes.
- Check if username is available: Yes.
- Account data: Yes.
- OpenID: No.

### Rooms
- Room creation: Yes, including presets.
- Joining rooms: Yes, including by alias or `?server_name=`.
- Event sending: Yes, including transaction IDs.
- Aliases: Yes.
- Published room directory: Yes.
- Kicking users: Yes.
- Banning users: Yes.
- Inviting users: Yes, but not third-party invites.
- Forgetting rooms: No.
- Room versions: All (v1 - v6)
- Tagging: Yes.

### User management
- User directory: Basic support.
- Ignoring users: No.
- Groups/Communities: No.

### Device management
- Creating devices: Yes.
- Deleting devices: Yes.
- Send-to-device messaging: Yes.

### Sync
- Filters: Timeline limit only. Rest unimplemented.
- Deprecated `/events` and `/initialSync`: No.

### Room events
- Typing: Yes.
- Receipts: No.
- Read Markers: No.
- Presence: No.
- Content repository (attachments): Yes.
- History visibility: No, defaults to `joined`.
- Push notifications: No.
- Event context: No.
- Reporting content: No.

### End-to-End Encryption
- Uploading device keys: Yes.
- Downloading device keys: Yes.
- Claiming one-time keys: Yes.
- Querying key changes: Yes.
- Cross-Signing: No.

### Misc
- Server-side search: No.
- Guest access: Partial.
- Room previews: No, partial support for Peeking via MSC2753.
- Third-Party networks: No.
- Server notices: No.
- Policy lists: No.

## Federation Features
- Querying keys (incl. notary): Yes.
- Server ACLs: Yes.
- Sending transactions: Yes.
- Joining rooms: Yes.
- Inviting to rooms: Yes, but not third-party invites.
- Leaving rooms: Yes.
- Content repository: Yes.
- Backfilling / get_missing_events: Yes.
- Retrieving state of the room (`/state` and `/state_ids`): Yes.
- Public rooms: Yes.
- Querying profile data: Yes.
- Device management: Yes.
- Send-to-Device messaging: Yes.
- Querying/Claiming E2E Keys: Yes.
- Typing: Yes.
- Presence: No.
- Receipts: No.
- OpenID: No.