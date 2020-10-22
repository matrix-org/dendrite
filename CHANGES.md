# Changelog

## Dendrite 0.2.1 (2020-10-22)

### Fixes

* Forward extremities are now calculated using only references from other extremities, rather than including outliers, which should fix cases where state can become corrupted ([#1556](https://github.com/matrix-org/dendrite/pull/1556))
* Old state events will no longer be processed by the sync API as new, which should fix some cases where clients incorrectly believe they have joined or left rooms ([#1548](https://github.com/matrix-org/dendrite/pull/1548))
* More SQLite database locking issues have been resolved in the latest events updater ([#1554](https://github.com/matrix-org/dendrite/pull/1554))
* Internal HTTP API calls are now made using H2C (HTTP/2) in polylith mode, mitigating some potential head-of-line blocking issues ([#1541](https://github.com/matrix-org/dendrite/pull/1541))
* Roomserver output events no longer incorrectly flag state rewrites ([#1557](https://github.com/matrix-org/dendrite/pull/1557))
* Notification levels are now parsed correctly in power level events ([gomatrixserverlib#228](https://github.com/matrix-org/gomatrixserverlib/pull/228), contributed by [Pestdoktor](https://github.com/Pestdoktor))
* Invalid UTF-8 is now correctly rejected when making federation requests ([gomatrixserverlib#229](https://github.com/matrix-org/gomatrixserverlib/pull/229), contributed by [Pestdoktor](https://github.com/Pestdoktor))

## Dendrite 0.2.0 (2020-10-20)

### Important

* This release makes breaking changes for polylith deployments, since they now use the multi-personality binary rather than separate binary files
  * Users of polylith deployments should revise their setups to use the new binary - see the Features section below
* This release also makes breaking changes for Docker deployments, as are now publishing images to Docker Hub in separate repositories for monolith and polylith
  * New repositories are as follows: [matrixdotorg/dendrite-monolith](https://hub.docker.com/repository/docker/matrixdotorg/dendrite-monolith) and [matrixdotorg/dendrite-polylith](https://hub.docker.com/repository/docker/matrixdotorg/dendrite-polylith)
  * The new `latest` tag will be updated with the latest release, and new versioned tags, e.g. `v0.2.0`, will preserve specific release versions
  * [Sample Compose configs](https://github.com/matrix-org/dendrite/tree/master/build/docker) have been updated - if you are running a Docker deployment, please review the changes
  * Images for the client API proxy and federation API proxy are no longer provided as they are unsupported - please use [nginx](docs/nginx/) (or another reverse proxy) instead

### Features

* Dendrite polylith deployments now use a special multi-personality binary, rather than separate binaries
  * This is cleaner, builds faster and simplifies deployment
  * The first command line argument states the component to run, e.g. `./dendrite-polylith-multi roomserver`
* Database migrations are now run at startup
* Invalid UTF-8 in requests is now rejected (contributed by [Pestdoktor](https://github.com/Pestdoktor))
* Fully read markers are now implemented in the client API (contributed by [Lesterpig](https://github.com/Lesterpig))
* Missing auth events are now retrieved from other servers in the room, rather than just the event origin
* `m.room.create` events are now validated properly when processing a `/send_join` response
* The roomserver now implements `KindOld` for handling historic events without them becoming forward extremity candidates, i.e. for backfilled or missing events

### Fixes

* State resolution v2 performance has been improved dramatically when dealing with large state sets
* The roomserver no longer processes outlier events if they are already known
* A SQLite locking issue in the previous events updater has been fixed
* The client API `/state` endpoint now correctly returns state after the leave event, if the user has left the room
* The client API `/createRoom` endpoint now sends cumulative state to the roomserver for the initial room events
* The federation API `/send` endpoint now correctly requests the entire room state from the roomserver when needed
* Some internal HTTP API paths have been fixed in the user API (contributed by [S7evinK](https://github.com/S7evinK))
* A race condition in the rate limiting code resulting in concurrent map writes has been fixed
* Each component now correctly starts a consumer/producer connection in monolith mode (when using Kafka)
* State resolution is no longer run for single trusted state snapshots that have been verified before
* A crash when rolling back the transaction in the latest events updater has been fixed
* Typing events are now ignored when the sender domain does not match the origin server
* Duplicate redaction entries no longer result in database errors
* Recursion has been removed from the code path for retrieving missing events
* `QueryMissingAuthPrevEvents` now returns events that have no associated state as if they are missing
* Signing key fetchers no longer ignore keys for the local domain, if retrieving a key that is not known in the local config
* Federation timeouts have been adjusted so we don't give up on remote requests so quickly
* `create-account` no longer relies on the device database (contributed by [ThatNerdyPikachu](https://github.com/ThatNerdyPikachu))

### Known issues

* Old events can incorrectly appear in `/sync` as if they are new when retrieving missing events from federated servers, causing them to appear at the bottom of the timeline in clients

## Dendrite 0.1.0 (2020-10-08)

First versioned release of Dendrite.

## Client-Server API Features

### Account registration and management

* Registration: By password only.
* Login: By password only. No fallback.
* Logout: Yes.
* Change password: Yes.
* Link email/msisdn to account: No.
* Deactivate account: Yes.
* Check if username is available: Yes.
* Account data: Yes.
* OpenID: No.

### Rooms

* Room creation: Yes, including presets.
* Joining rooms: Yes, including by alias or `?server_name=`.
* Event sending: Yes, including transaction IDs.
* Aliases: Yes.
* Published room directory: Yes.
* Kicking users: Yes.
* Banning users: Yes.
* Inviting users: Yes, but not third-party invites.
* Forgetting rooms: No.
* Room versions: All (v1 * v6)
* Tagging: Yes.

### User management

* User directory: Basic support.
* Ignoring users: No.
* Groups/Communities: No.

### Device management

* Creating devices: Yes.
* Deleting devices: Yes.
* Send-to-device messaging: Yes.

### Sync

* Filters: Timeline limit only. Rest unimplemented.
* Deprecated `/events` and `/initialSync`: No.

### Room events

* Typing: Yes.
* Receipts: No.
* Read Markers: No.
* Presence: No.
* Content repository (attachments): Yes.
* History visibility: No, defaults to `joined`.
* Push notifications: No.
* Event context: No.
* Reporting content: No.

### End-to-End Encryption

* Uploading device keys: Yes.
* Downloading device keys: Yes.
* Claiming one-time keys: Yes.
* Querying key changes: Yes.
* Cross-Signing: No.

### Misc

* Server-side search: No.
* Guest access: Partial.
* Room previews: No, partial support for Peeking via MSC2753.
* Third-Party networks: No.
* Server notices: No.
* Policy lists: No.

## Federation Features

* Querying keys (incl. notary): Yes.
* Server ACLs: Yes.
* Sending transactions: Yes.
* Joining rooms: Yes.
* Inviting to rooms: Yes, but not third-party invites.
* Leaving rooms: Yes.
* Content repository: Yes.
* Backfilling / get_missing_events: Yes.
* Retrieving state of the room (`/state` and `/state_ids`): Yes.
* Public rooms: Yes.
* Querying profile data: Yes.
* Device management: Yes.
* Send-to-Device messaging: Yes.
* Querying/Claiming E2E Keys: Yes.
* Typing: Yes.
* Presence: No.
* Receipts: No.
* OpenID: No.