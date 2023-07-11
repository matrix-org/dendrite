# Changelog

## Dendrite 0.13.1 (2023-07-06)

This releases fixes a long-standing "off-by-one" error which could result in state resets. Upgrading to this version is **highly** recommended.

When deduplicating state events, we were checking if the event in question was already in a state snapshot. If it was in a previous state snapshot, we would 
then remove it from the list of events to store. If this happened, we were, unfortunately, skipping the next event to check. This resulted in 
events getting stored in state snapshots where they may not be needed. When we now compared two of those state snapshots, one of them
contained the skipped event, while the other didn't. This difference possibly shouldn't exist, resulting in unexpected state resets and explains
reports of missing state events as well.

Rooms where a state reset occurred earlier should, hopefully, reconcile over time.

### Fixes:

- A long-standing "off-by-one" error has been fixed, which could result in state resets
- Roomserver Prometheus Metrics are available again

### Features

- Updated dependencies
  - Internal NATS Server has been updated from v2.9.15 to v2.9.19

## Dendrite 0.13.0 (2023-06-30)

### Features

- Results in responses to `/search` now highlight words more accurately and not only the search terms as before
- Support for connecting to appservices listening on unix sockets has been added (contributed by [cyberb](https://github.com/cyberb))
- Admin APIs for token authenticated registration have been added (contributed by [santhoshivan23](https://github.com/santhoshivan23))
- Initial support for [MSC4014: Pseudonymous Identities](https://github.com/matrix-org/matrix-spec-proposals/blob/kegan/pseudo-ids/proposals/4014-pseudonymous-identities.md)
  - This is **highly experimental**, things like changing usernames/avatars, inviting users, upgrading rooms isn't working

### Fixes

- `m.upload.size` is now optional, finally allowing uploads with unlimited file size
- A bug while resolving server names has been fixed (contributed by [anton-molyboha](https://github.com/anton-molyboha))
- Application services should only receive one invitation instead of 2 (or worse), which could result in state resets previously
- Several admin endpoints are now using `POST` instead of `GET`
- `/delete_devices` now uses user-interactive authentication
- Several "membership" (e.g `/kick`, `/ban`) endpoints are using less heavy database queries to check if the user is allowed to perform this action
- `/3pid` endpoints are now available on `/v3` instead of the `/unstable` prefix
- Upgrading rooms ignores state events of other users, which could result in failed upgrades before
- Uploading key backups with a wrong version now returns `M_WRONG_ROOM_KEYS_VERSION`
- A potential state reset when joining the same room multiple times in short sequence has been fixed
- A bug where we returned the full event as `redacted_because` in redaction events has been fixed
- The `displayname` and `avatar_url` can now be set to empty strings
- Unsafe hotserving of files has been fixed (contributed by [joshqou](https://github.com/joshqou))
- Joining new rooms would potentially return "redacted" events, due to history visibility not being set correctly, this could result in events being rejected 
- Backfilling resulting in `unsuported room version ''` should now be solved

### Other

- Huge refactoring of Dendrite and gomatrixserverlib

## Dendrite 0.12.0 (2023-03-13)

### Features

- The userapi and keyserver have been merged (no actions needed regarding the database)
- The internal NATS JetStream server is now using logrus for logging (contributed by [dvob](https://github.com/dvob))
- The roomserver database has been refactored to have separate interfaces when working with rooms and events. Also includes increased usage of the cache to avoid database round trips. (database is unchanged)
- The pinecone demo now shuts down more cleanly
- The Helm chart now has the ability to deploy a Grafana chart as well (contributed by [genofire](https://github.com/genofire))
- Support for listening on unix sockets has been added (contributed by [cyberb](https://github.com/cyberb))
- The internal NATS server was updated to v2.9.15
- Initial support for `runtime/trace` has been added, to further track down long-running tasks

### Fixes

- The `session_id` is now correctly set when using SQLite
- An issue where device keys could be removed if a device ID is reused has been fixed
- A possible DoS issue related to relations has been fixed (reported by [sleroq](https://github.com/sleroq))
- When backfilling events, errors are now ignored if we still could fetch events

### Other

- **⚠️ DEPRECATION: Polylith/HTTP API mode has been removed**
- The default endpoint to report usages stats to has been updated

## Dendrite 0.11.1 (2023-02-10)

**⚠️ DEPRECATION WARNING: This is the last release to have polylith and HTTP API mode. Future releases are monolith only.**

### Features

* Dendrite can now be compiled against Go 1.20
* Initial store and forward support has been added
* A landing page showing that Dendrite is running has been added (contributed by [LukasLJL](https://github.com/LukasLJL))

### Fixes

- `/sync` is now using significantly less database round trips when using Postgres, resulting in faster initial syncs, allowing larger accounts to login again
- Many under the hood pinecone improvements
- Publishing rooms is now possible again

## Dendrite 0.11.0 (2023-01-20)

The last three missing federation API Sytests have been fixed - bringing us to 100% server-server Synapse parity, with client-server parity at 93% 🎉

### Features

* Added `/_dendrite/admin/purgeRoom/{roomID}` to clean up the database
* The default room version was updated to 10 (contributed by [FSG-Cat](https://github.com/FSG-Cat))

### Fixes

* An oversight in the `create-config` binary, which now correctly sets the media path if specified (contributed by [BieHDC](https://github.com/BieHDC))
* The Helm chart now uses the `$.Chart.AppVersion` as the default image version to pull, with the possibility to override it (contributed by [genofire](https://github.com/genofire))

## Dendrite 0.10.9 (2023-01-17)

### Features

* Stale device lists are now cleaned up on startup, removing entries for users the server doesn't share a room with anymore
* Dendrite now has its own Helm chart
* Guest access is now handled correctly (disallow joins, kick guests on revocation of guest access, as well as over federation)

### Fixes

* Push rules have seen several tweaks and fixes, which should, for example, fix notifications for `m.read_receipts`
* Outgoing presence will now correctly be sent to newly joined hosts
* Fixes the `/_dendrite/admin/resetPassword/{userID}` admin endpoint to use the correct variable
* Federated backfilling for medium/large rooms has been fixed
* `/login` causing wrong device list updates has been resolved
* `/sync` should now return the correct room summary heroes
* The default config options for `recaptcha_sitekey_class` and `recaptcha_form_field` are now set correctly 
* `/messages` now omits empty `state` to be more spec compliant (contributed by [handlerug](https://github.com/handlerug))
* `/sync` has been optimised to only query state events for history visibility if they are really needed

## Dendrite 0.10.8 (2022-11-29)

### Features

* The built-in NATS Server has been updated to version 2.9.8
* A number of under-the-hood changes have been merged for future virtual hosting support in Dendrite (running multiple domain names on the same Dendrite deployment)

### Fixes

* Event auth handling of invites has been refactored, which should fix some edge cases being handled incorrectly
* Fix a bug when returning an empty protocol list, which could cause Element to display "The homeserver may be too old to support third party networks" when opening the public room directory
* The sync API will no longer filter out the user's own membership when using lazy-loading
* Dendrite will now correctly detect JetStream consumers being deleted, stopping the consumer goroutine as needed
* A panic in the federation API where the server list could go out of bounds has been fixed
* Blacklisted servers will now be excluded when querying joined servers, which improves CPU usage and performs less unnecessary outbound requests
* A database writer will now be used to assign state key NIDs when requesting NIDs that may not exist yet
* Dendrite will now correctly move local aliases for an upgraded room when the room is upgraded remotely
* Dendrite will now correctly move account data for an upgraded room when the room is upgraded remotely
* Missing state key NIDs will now be allocated on request rather than returning an error
* Guest access is now correctly denied on a number of endpoints
* Presence information will now be correctly sent for new private chats
* A number of unspecced fields have been removed from outbound `/send` transactions

## Dendrite 0.10.7 (2022-11-04)

### Features

* Dendrite will now use a native SQLite port when building with `CGO_ENABLED=0`
* A number of `thirdparty` endpoints have been added, improving support for appservices

### Fixes

* The `"state"` section of the `/sync` response is no longer limited, so state events should not be dropped unexpectedly
* The deduplication of the `"timeline"` and `"state"` sections in `/sync` is now performed after applying history visibility, so state events should not be dropped unexpectedly
* The `prev_batch` token returned by `/sync` is now calculated after applying history visibility, so that the pagination boundaries are correct
* The room summary membership counts in `/sync` should now be calculated properly in more cases
* A false membership leave event should no longer be sent down `/sync` as a result of retiring an accepted invite (contributed by [tak-hntlabs](https://github.com/tak-hntlabs))
* Presence updates are now only sent to other servers for which the user shares rooms
* A bug which could cause a panic when converting events into the `ClientEvent` format has been fixed

## Dendrite 0.10.6 (2022-11-01)

### Features

* History visibility checks have been optimised, which should speed up response times on a variety of endpoints (including `/sync`, `/messages`, `/context` and others) and reduce database load
* The built-in NATS Server has been updated to version 2.9.4
* Some other minor dependencies have been updated

### Fixes

* A panic has been fixed in the sync API PDU stream which could cause requests to fail
* The `/members` response now contains the `room_id` field, which may fix some E2EE problems with clients using the JS SDK (contributed by [ashkitten](https://github.com/ashkitten))
* The auth difference calculation in state resolution v2 has been tweaked for clarity (and moved into gomatrixserverlib with the rest of the state resolution code)

## Dendrite 0.10.5 (2022-10-31)

### Features

* It is now possible to use hCaptcha instead of reCAPTCHA for protecting registration
* A new `auto_join_rooms` configuration option has been added for automatically joining new users to a set of rooms
* A new `/_dendrite/admin/downloadState/{serverName}/{roomID}` endpoint has been added, which allows a server administrator to attempt to repair a room with broken room state by downloading a state snapshot from another federated server in the room

### Fixes

* Querying cross-signing keys for users should now be considerably faster
* A bug in state resolution where some events were not correctly selected for third-party invites has been fixed
* A bug in state resolution which could result in `not in room` event rejections has been fixed
* When accepting a DM invite, it should now be possible to see messages that were sent before the invite was accepted
* Claiming remote E2EE one-time keys has been refactored and should be more reliable now
* Various fixes have been made to the `/members` endpoint, which may help with E2EE reliability and clients rendering memberships
* A race condition in the federation API destination queues has been fixed when associating queued events with remote server destinations
* A bug in the sync API where too many events were selected resulting in high CPU usage has been fixed
* Configuring the avatar URL for the Server Notices user should work correctly now

## Dendrite 0.10.4 (2022-10-21)

### Features

* Various tables belonging to the user API will be renamed so that they are namespaced with the `userapi_` prefix
  * Note that, after upgrading to this version, you should not revert to an older version of Dendrite as the database changes **will not** be reverted automatically
* The backoff and retry behaviour in the federation API has been refactored and improved

### Fixes

* Private read receipt support is now advertised in the client `/versions` endpoint
* Private read receipts will now clear notification counts properly
* A bug where a false `leave` membership transition was inserted into the timeline after accepting an invite has been fixed
* Some panics caused by concurrent map writes in the key server have been fixed
* The sync API now calculates membership transitions from state deltas more accurately
* Transaction IDs are now scoped to endpoints, which should fix some bugs where transaction ID reuse could cause nonsensical cached responses from some endpoints
* The length of the `type`, `sender`, `state_key` and `room_id` fields in events are now verified by number of bytes rather than codepoints after a spec clarification, reverting a change made in Dendrite 0.9.6

## Dendrite 0.10.3 (2022-10-14)

### Features

* Event relations are now tracked and support for the `/room/{roomID}/relations/...` client API endpoints have been added
* Support has been added for private read receipts
* The built-in NATS Server has been updated to version 2.9.3

### Fixes

* The `unread_notifications` are now always populated in joined room responses
* The `/get_missing_events` federation API endpoint should now work correctly for rooms with `joined` and `invited` visibility settings, returning redacted events for events that other servers are not allowed to see
* The `/event` client API endpoint now applies history visibility correctly
* Read markers should now be updated much more reliably
* A rare bug in the sync API which could cause some `join` memberships to be incorrectly overwritten by other memberships when working out which rooms to populate has been fixed
* The federation API now correctly updates the joined hosts table during a state rewrite

## Dendrite 0.10.2 (2022-10-07)

### Features

* Dendrite will now fail to start if there is an obvious problem with the configured `max_open_conns` when using PostgreSQL database backends, since this can lead to instability and performance issues
  * More information on this is available [in the documentation](https://matrix-org.github.io/dendrite/installation/start/optimisation#postgresql-connection-limit)
* Unnecessary/empty fields will no longer be sent in `/sync` responses
* It is now possible to configure `old_private_keys` from previous Matrix installations on the same domain if only public key is known, to make it easier to expire old keys correctly
  * You can configure either just the `private_key` path, or you can supply both the `public_key` and `key_id`

### Fixes

* The sync transaction behaviour has been modified further so that errors in one stream should not propagate to other streams unnecessarily
* Rooms should now be classified as DM rooms correctly by passing through `is_direct` and unsigned hints
* A bug which caused marking device lists as stale to consume lots of CPU has been fixed
* Users accepting invites should no longer cause unnecessary federated joins if there are already other local users in the room
* The sync API state range queries have been optimised by adding missing indexes
* It should now be possible to configure non-English languages for full-text search in `search.language`
* The roomserver will no longer attempt to perform federated requests to the local server when trying to fetch missing events
* The `/keys/upload` endpoint will now always return the `one_time_keys_counts`, which may help with E2EE reliability
* The sync API will now retrieve the latest stream position before processing each stream rather than at the beginning of the request, to hopefully reduce the number of round-trips to `/sync`

## Dendrite 0.10.1 (2022-09-30)

### Features

* The built-in NATS Server has been updated to version 2.9.2

### Fixes

* A regression introduced in 0.10.0 in `/sync` as a result of transaction errors has been fixed
* Account data updates will no longer send duplicate output events

## Dendrite 0.10.0 (2022-09-30)

### Features

* High performance full-text searching has been added to Dendrite
  * Search must be enabled in the [`search` section of the `sync_api` config](https://github.com/matrix-org/dendrite/blob/6348486a1365c7469a498101f5035a9b6bd16d22/dendrite-sample.monolith.yaml#L279-L290) before it can be used
  * The search index is stored on the filesystem rather than the sync API database, so a path to a suitable storage location on disk must be configured
* Sync requests should now complete faster and use considerably less database connections as a result of better transactional isolation
* The notifications code has been refactored to hopefully make notifications more reliable
* A new `/_dendrite/admin/refreshDevices/{userID}` admin endpoint has been added for forcing a refresh of a remote user's device lists without having to modify the database by hand
* A new `/_dendrite/admin/fulltext/reindex` admin endpoint has been added for rebuilding the search index (although this may take some time)

### Fixes

* A number of bugs in the device list updater have been fixed, which should help considerably with federated device list synchronisation and E2EE reliability
* A state resolution bug has been fixed which should help to prevent unexpected state resets
* The deprecated `"origin"` field in events will now be correctly ignored in all cases
* Room versions 8 and 9 will now correctly evaluate `"knock"` join rules and membership states
* A database index has been added to speed up finding room memberships in the sync API (contributed by [PiotrKozimor](https://github.com/PiotrKozimor))
* The client API will now return an `M_UNRECOGNIZED` error for unknown endpoints/methods, which should help with client error handling
* A bug has been fixed when updating push rules which could result in `database is locked` on SQLite

## Dendrite 0.9.9 (2022-09-22)

### Features

* Dendrite will now try to keep HTTP connections open to remote federated servers for a few minutes after a request and attempt to reuse those connections where possible
  * This should reduce the amount of time spent on TLS handshakes and often speed up requests to remote servers
  * This new behaviour can be disabled with the `federation_api.disable_http_keepalives` option if needed
* A number of dependencies have been updated

### Fixes

* A bug where the roomserver did not correctly propagate rewritten room state to downstream components (like the federation API and sync API) has been fixed, which could cause issues when performing a federated join to a previously left room
* Event auth now correctly parses the `join_authorised_via_users_server` field in the membership event content
* Database migrations should no longer produce unique constraint errors at Dendrite startup
* The `origin` of device list updates should now be populated correctly
* Send-to-device messages will no longer be dropped if we fail to publish them to specific devices
* The roomserver query to find state after events will now always resolve state if there are multiple prev events
* The roomserver will now return no memberships if querying history visibility for an event which has no state snapshot
* The device list updater will now mark a device list as stale if a requesting device ID is not known
* Transactions sent to appservices should no longer have accidental duplicated transaction IDs (contributed by [tak-hntlabs](https://github.com/tak-hntlabs))

## Dendrite 0.9.8 (2022-09-12)

### Important

* This is a **security release** to fix a vulnerability where missing events retrieved from other servers did not have their signatures verified in all cases, affecting all versions of Dendrite before 0.9.8. Upgrading to this version is highly recommended. For more information, [see here](https://github.com/matrix-org/dendrite/security/advisories/GHSA-pfw4-xjgm-267c).

### Features

* The built-in NATS Server has been updated to the final 2.9.0 release version

### Fixes

* Dendrite will now correctly verify the signatures of events retrieved using `/get_missing_events`

## Dendrite 0.9.7 (2022-09-09)

### Features

* Initial supporting code to enable full-text search has been merged (although not ready for use yet)
* Newly created rooms now have higher default power levels for enabling encryption, setting server ACLs or sending tombstone events
* Incoming signing key updates over federation are now queued in JetStream for processing, so that they cannot be dropped accidentally

### Fixes

* A race condition between the roomserver output events being generated, forward extremities being updated and room info being updated has been fixed
* Appservices will no longer receive invite events which they are not interested in, which caused heavy load in some cases or excessive request sizes in others
* A bug in state resolution v2 where events could incorrectly be classified as control events has been fixed
* A bug in state resolution v2 where some specific events with unexpected non-empty state keys are dropped has been fixed
* A bug in state resolution v2 when fetching auth events vs partial state has been fixed
* Stale device lists should now be handled correctly for all user IDs, which may help with E2EE reliability
* A number of database writer issues have been fixed in the user API and sync API, which should help to reduce `database is locked` errors with SQLite databases
* Database migrations should now be detected more reliably to prevent unexpected errors at startup
* A number of minor database transaction issues have been fixed, particularly for assigning NIDs in the roomserver, cleaning up device keys and cleaning up notifications
* The database query for finding shared users in the sync API has been optimised, using significantly less CPU time as a result

## Dendrite 0.9.6 (2022-09-01)

### Features

* The appservice API has been refactored for improved performance and stability
  * The appservice database has been deprecated, as the roomserver output stream is now used as the data source instead
* The `generate-config` tool has been updated to support additional scenarios, i.e. for CI configuration generation and generating both monolith and polylith skeleton config files

### Fixes

* The username length check has been fixed on new account creation
* The length of the `type`, `sender`, `state_key` and `room_id` fields in events are now verified by number of codepoints rather than bytes, fixing the "Cat Overflow" bug
* UTF-16 surrogate handling in the canonical JSON implementation has been fixed
* A race condition when starting the keyserver has been fixed
* A race condition when configuring HTTP servers and routing at startup has been fixed
* A bug where the incorrect limit was used for lazy-loading memberships has been fixed
* The number of push notifications will now be sent to the push gateway
* A missing index causing slow performance on the sync API send-to-device table has been added (contributed by [PiotrKozimor](https://github.com/PiotrKozimor))
* Event auth will now correctly check for the existence of the `"creator"` field in create events

## Dendrite 0.9.5 (2022-08-25)

### Fixes

* The roomserver will now correctly unreject previously rejected events if necessary when reprocessing
* The handling of event soft-failure has been improved on the roomserver input by no longer applying rejection rules and still calculating state before the event if possible
* The federation `/state` and `/state_ids` endpoints should now return the correct error code when the state isn't known instead of returning a HTTP 500
* The federation `/event` should now return outlier events correctly instead of returning a HTTP 500
* A bug in the federation backoff allowing zero intervals has been corrected
* The `create-account` utility will no longer error if the homeserver URL ends in a trailing slash
* A regression in `/sync` introduced in 0.9.4 should be fixed

## Dendrite 0.9.4 (2022-08-19)

### Fixes

* A bug in the roomserver around handling rejected outliers has been fixed
* Backfilled events will now use the correct history visibility where possible
* The device list updater backoff has been fixed, which should reduce the number of outbound HTTP requests and `Failed to query device keys for some users` log entries for dead servers
* The `/sync` endpoint will no longer incorrectly return room entries for retired invites which could cause some rooms to show up in the client "Historical" section
* The `/createRoom` endpoint will now correctly populate `is_direct` in invite membership events, which may help clients to classify direct messages correctly
* The `create-account` tool will now log an error if the shared secret is not set in the Dendrite config
* A couple of minor bugs have been fixed in the membership lazy-loading
* Queued EDUs in the federation API are now cached properly

## Dendrite 0.9.3 (2022-08-15)

### Important

* This is a **security release** to fix a vulnerability within event auth, affecting all versions of Dendrite before 0.9.3. Upgrading to this version is highly recommended. For more information, [see here](https://github.com/matrix-org/gomatrixserverlib/security/advisories/GHSA-grvv-h2f9-7v9c).

### Fixes

* Dendrite will now correctly parse the `"events_default"` power level value for event auth.

## Dendrite 0.9.2 (2022-08-12)

### Features

* Dendrite now supports history visibility on the `/sync`, `/messages` and `/context` endpoints
  * It should now be possible to view the history of a room in more cases (as opposed to limiting scrollback to the join event or defaulting to the restrictive `"join"` visibility rule as before)
* The default room version for newly created rooms is now room version 9
* New admin endpoint `/_dendrite/admin/resetPassword/{userID}` has been added, which replaces the `-reset-password` flag in `create-account`
* The `create-account` binary now uses shared secret registration over HTTP to create new accounts, which fixes a number of problems with account data and push rules not being configured correctly for new accounts
* The internal HTTP APIs for polylith deployments have been refactored for correctness and consistency
* The federation API will now automatically clean up some EDUs that have failed to send within a certain period of time
* The `/hierarchy` endpoint will now return potentially joinable rooms (contributed by [texuf](https://github.com/texuf))
* The user directory will now show or hide users correctly

### Fixes

* Send-to-device messages should no longer be incorrectly duplicated in `/sync`
* The federation sender will no longer create unnecessary destination queues as a result of a logic error
* A bug where database migrations may not execute properly when upgrading from older versions has been fixed
* A crash when failing to update user account data has been fixed
* A race condition when generating notification counts has been fixed
* A race condition when setting up NATS has been fixed (contributed by [brianathere](https://github.com/brianathere))
* Stale cache data for membership lazy-loading is now correctly invalidated when doing a complete sync
* Data races within user-interactive authentication have been fixed (contributed by [tak-hntlabs](https://github.com/tak-hntlabs))

## Dendrite 0.9.1 (2022-08-03)

### Fixes

* Upgrades a dependency which caused issues building Dendrite with Go 1.19
* The roomserver will no longer give up prematurely after failing to call `/state_ids`
* Removes the faulty room info cache, which caused of a number of race conditions and occasional bugs (including when creating and joining rooms)
* The media endpoint now sets the `Cache-Control` header correctly to prevent web-based clients from hitting media endpoints excessively
* The sync API will now advance the PDU stream position correctly in all cases (contributed by [sergekh2](https://github.com/sergekh2))
* The sync API will now delete the correct range of send-to-device messages when advancing the stream position
* The device list `changed` key in the `/sync` response should now return the correct users
* A data race when looking up missing state has been fixed
* The `/send_join` API is now applying stronger validation to the received membership event

## Dendrite 0.9.0 (2022-08-01)

### Features

* Dendrite now uses Ristretto for managing in-memory caches
  * Should improve cache utilisation considerably over time by more intelligently selecting and managing cache entries compared to the previous LRU-based cache
  * Defaults to a 1GB cache size if not configured otherwise
  * The estimated cache size in memory and maximum age can now be configured with new [configuration options](https://github.com/matrix-org/dendrite/blob/e94ef84aaba30e12baf7f524c4e7a36d2fdeb189/dendrite-sample.monolith.yaml#L44-L61) to prevent unbounded cache growth
* Added support for serving the `/.well-known/matrix/client` hint directly from Dendrite
  * Configurable with the new [configuration option](https://github.com/matrix-org/dendrite/blob/e94ef84aaba30e12baf7f524c4e7a36d2fdeb189/dendrite-sample.monolith.yaml#L67-L69)
* Refactored membership updater, which should eliminate some bugs caused by the membership table getting out of sync with the room state
* The User API is now responsible for sending account data updates to other components, which may fix some races and duplicate account data events
* Optimised database query for checking whether a remote server is allowed to request an event over federation without using anywhere near as much CPU time (PostgreSQL only)
* Database migrations have been refactored to eliminate some problems that were present with `goose` and upgrading from older Dendrite versions
* Media fetching will now use the `/v3` endpoints for downloading media from remote homeservers
* HTTP 404 and HTTP 405 errors from the client-facing APIs should now be returned with CORS headers so that web-based clients do not produce incorrect access control warnings for unknown endpoints
* Some preparation work for full history visibility support

### Fixes

* Fixes a crash that could occur during event redaction
* The `/members` endpoint will no longer incorrectly return HTTP 500 as a result of some invite events
* Send-to-device messages should now be ordered more reliably and the last position in the stream updated correctly
* Parsing of appservice configuration files is now less strict (contributed by [Kab1r](https://github.com/Kab1r))
* The sync API should now identify shared users correctly when waking up for E2EE key changes
* The federation `/state` endpoint will now return a HTTP 403 when the state before an event isn't known instead of a HTTP 500
* Presence timestamps should now be calculated with the correct precision
* A race condition in the roomserver's room info has been fixed
* A race condition in the sync API has been fixed

## Dendrite 0.8.9 (2022-07-01)

### Features

* Incoming device list updates over federation are now queued in JetStream for processing so that they will no longer block incoming federation transactions and should never end up dropped, which will hopefully help E2EE reliability
* The `/context` endpoint now returns `"start"` and `"end"` parameters to allow pagination from a context call
* The `/messages` endpoint will no longer return `"end"` when there are no more messages remaining
* Deactivated user accounts will now leave all rooms automatically
* New admin endpoint `/_dendrite/admin/evacuateUser/{userID}` has been added for forcing a local user to leave all joined rooms
* Dendrite will now automatically attempt to raise the file descriptor limit at startup if it is too low

### Fixes

* A rare crash when retrieving remote device lists has been fixed
* Fixes a bug where events were not redacted properly over federation
* The `/invite` endpoints will now return an error instead of silently proceeding if the user ID is obviously malformed

## Dendrite 0.8.8 (2022-06-09)

### Features

* The performance of state resolution has been increased significantly for larger rooms
* A number of changes have been made to rate limiting:
  * Logged in users will now be rate-limited on a per-session basis rather than by remote IP
  * Rate limiting no longer applies to admin or appservice users
  * It is now possible to configure additional users that are exempt from rate limiting using the `exempt_user_ids` option in the `rate_limiting` section of the Dendrite config
* Setting state is now idempotent via the client API state endpoints

### Fixes

* Room upgrades now properly propagate tombstone events to remote servers
* Room upgrades will no longer send tombstone events if creating the upgraded room fails
* A crash has been fixed when evaluating restricted room joins

## Dendrite 0.8.7 (2022-06-01)

### Features

* Support added for room version 10

### Fixes

* A number of state handling bugs have been fixed, which previously resulted in missing state events, unexpected state deletions, reverted memberships and unexpectedly rejected/soft-failed events in some specific cases
* Fixed destination queue performance issues as a result of missing indexes, which speeds up outbound federation considerably
* A bug which could cause the `/register` endpoint to return HTTP 500 has been fixed

## Dendrite 0.8.6 (2022-05-26)

### Features

* Room versions 8 and 9 are now marked as stable
* Dendrite can now assist remote users to join restricted rooms via `/make_join` and `/send_join`

### Fixes

* The sync API no longer returns immediately on `/sync` requests unnecessarily if it can be avoided
* A race condition has been fixed in the sync API when updating presence via `/sync`
* A race condition has been fixed sending E2EE keys to remote servers over federation when joining rooms
* The `trusted_private_chat` preset should now grant power level 100 to all participant users, which should improve the user experience of direct messages
* Invited users are now authed correctly in restricted rooms
* The `join_authorised_by_users_server` key is now correctly stripped in restricted rooms when updating the membership event
* Appservices should now receive invite events correctly
* Device list updates should no longer contain optional fields with `null` values
* The `/deactivate` endpoint has been fixed to no longer confuse Element with incorrect completed flows

## Dendrite 0.8.5 (2022-05-13)

### Features

* New living documentation available at <https://matrix-org.github.io/dendrite/>, including new installation instructions
* The built-in NATS Server has been updated to version 2.8.2

### Fixes

* Monolith deployments will no longer panic at startup if given a config file that does not include the `internal_api` and `external_api` options
* State resolution v2 now correctly identifies other events related to power events, which should fix some event auth issues
* The latest events updater will no longer implicitly trust the new forward extremities when calculating the current room state, which may help to avoid some state resets
* The one-time key count is now correctly returned in `/sync` even if the request otherwise timed out, which should reduce the chance that unnecessary one-time keys will be uploaded by clients
* The `create-account` tool should now work properly when the database is configured using the global connection pool

## Dendrite 0.8.4 (2022-05-10)

### Fixes

* Fixes a regression introduced in the previous version where appservices, push and phone-home statistics would not work over plain HTTP
* Adds missing indexes to the sync API output events table, which should significantly improve `/sync` performance and reduce database CPU usage
* Building Dendrite with the `bimg` thumbnailer should now work again (contributed by [database64128](https://github.com/database64128))

## Dendrite 0.8.3 (2022-05-09)

### Features

* Open registration is now harder to enable, which should reduce the chance that Dendrite servers will be used to conduct spam or abuse attacks
  * Dendrite will only enable open registration if you pass the `--really-enable-open-registration` command line flag at startup
  * If open registration is enabled but this command line flag is not passed, Dendrite will fail to start up
* Dendrite now supports phone-home statistic reporting
  * These statistics include things like the number of registered and active users, some configuration options and platform/environment details, to help us to understand how Dendrite is used
  * This is not enabled by default — it must be enabled in the `global.report_stats` section of the config file
* Monolith installations can now be configured with a single global database connection pool (in `global.database` in the config) rather than having to configure each component separately
  * This also means that you no longer need to balance connection counts between different components, as they will share the same larger pool
  * Specific components can override the global database settings by specifying their own `database` block
  * To use only the global pool, you must configure `global.database` and then remove the `database` block from all of the component sections of the config file
* A new admin API endpoint `/_dendrite/admin/evacuateRoom/{roomID}` has been added, allowing server admins to forcefully part all local users from a given room
* The sync notifier now only loads members for the relevant rooms, which should reduce CPU usage and load on the database
* A number of component interfaces have been refactored for cleanliness and developer ease
* Event auth errors in the log should now be much more useful, including the reason for the event failures
* The forward extremity calculation in the roomserver has been simplified
* A new index has been added to the one-time keys table in the keyserver which should speed up key count lookups

### Fixes

* Dendrite will no longer process events for rooms where there are no local users joined, which should help to reduce CPU and RAM usage
* A bug has been fixed in event auth when changing the user levels in `m.room.power_levels` events
* Usernames should no longer be duplicated when no room name is set
* Device display names should now be correctly propagated over federation
* A panic when uploading cross-signing signatures has been fixed
* Presence is now correctly limited in `/sync` based on the filters
* The presence stream position returned by `/sync` will now be correct if no presence events were returned
* The media `/config` endpoint will no longer return a maximum upload size field if it is configured to be unlimited in the Dendrite config
* The server notices room will no longer produce "User is already joined to the room" errors
* Consumer errors will no longer flood the logs during a graceful shutdown
* Sync API and federation API consumers will no longer unnecessarily query added state events matching the one in the output event
* The Sync API will no longer unnecessarily track invites for remote users

## Dendrite 0.8.2 (2022-04-27)

### Features

* Lazy-loading has been added to the `/sync` endpoint, which should speed up syncs considerably
* Filtering has been added to the `/messages` endpoint
* The room summary now contains "heroes" (up to 5 users in the room) for clients to display when no room name is set
* The existing lazy-loading caches will now be used by `/messages` and `/context` so that member events will not be sent to clients more times than necessary
* The account data stream now uses the provided filters
* The built-in NATS Server has been updated to version 2.8.0
* The `/state` and `/state_ids` endpoints will now return `M_NOT_FOUND` for rejected events
* Repeated calls to the `/redact` endpoint will now be idempotent when a transaction ID is given
* Dendrite should now be able to run as a Windows service under Service Control Manager

### Fixes

* Fictitious presence updates will no longer be created for users which have not sent us presence updates, which should speed up complete syncs considerably
* Uploading cross-signing device signatures should now be more reliable, fixing a number of bugs with cross-signing
* All account data should now be sent properly on a complete sync, which should eliminate problems with client settings or key backups appearing to be missing
* Account data will now be limited correctly on incremental syncs, returning the stream position of the most recent update rather than the latest stream position
* Account data will not be sent for parted rooms, which should reduce the number of left/forgotten rooms reappearing in clients as empty rooms
* The TURN username hash has been fixed which should help to resolve some problems when using TURN for voice calls (contributed by [fcwoknhenuxdfiyv](https://github.com/fcwoknhenuxdfiyv))
* Push rules can no longer be modified using the account data endpoints
* Querying account availability should now work properly in polylith deployments
* A number of bugs with sync filters have been fixed
* A default sync filter will now be used if the request contains a filter ID that does not exist
* The `pushkey_ts` field is now using seconds instead of milliseconds
* A race condition when gracefully shutting down has been fixed, so JetStream should no longer cause the process to exit before other Dendrite components are finished shutting down

## Dendrite 0.8.1 (2022-04-07)

### Fixes

* A bug which could result in the sync API deadlocking due to lock contention in the notifier has been fixed

## Dendrite 0.8.0 (2022-04-07)

### Features

* Support for presence has been added
  * Presence is not enabled by default
  * The `global.presence.enable_inbound` and `global.presence.enable_outbound` configuration options allow configuring inbound and outbound presence separately
* Support for room upgrades via the `/room/{roomID}/upgrade` endpoint has been added (contributed by [DavidSpenler](https://github.com/DavidSpenler), [alexkursell](https://github.com/alexkursell))
* Support for ignoring users has been added
* Joined and invite user counts are now sent in the `/sync` room summaries
* Queued federation and stale device list updates will now be staggered at startup over an up-to 2 minute warm-up period, rather than happening all at once
* Memory pressure created by the sync notifier has been reduced
* The EDU server component has now been removed, with the work being moved to more relevant components

### Fixes

* It is now possible to set the `power_level_content_override` when creating a room to include power levels over 100
* `/send_join` and `/state` responses will now not unmarshal the JSON twice
* The stream event consumer for push notifications will no longer request membership events that are irrelevant
* Appservices will no longer incorrectly receive state events twice

## Dendrite 0.7.0 (2022-03-25)

### Features

* The roomserver input API will now queue all events into NATS, which provides better crash resilience
* The roomserver input API now configures per-room consumers, which should use less memory
* Canonical aliases can now be added and removed
* MSC2946 Spaces Summary now works correctly, both locally and over federation
* Healthcheck endpoints are now available at:
  * `/_dendrite/monitor/up`, which will return 200 when Dendrite is ready to accept requests
  * `/_dendrite/monitor/health`, which will return 200 if healthy and 503 if degraded for some reason
* The `X-Matrix` federation authorisation header now includes a `destination` field, as per MSC3383
* The `/sync` endpoint now uses less memory by only ranging state for rooms that the user has participated in
* The `/messages` endpoint now accepts stream positions in both the `from` and `to` parameters
* Dendrite will now log a warning at startup if the file descriptor limit is set too low
* The federation client will now attempt to use HTTP/2 if available
* The federation client will now attempt to resume TLS sessions if possible, to reduce handshake overheads
* The built-in NATS Server has been updated to version 2.7.4
* NATS streams that don't match the desired configuration will now be recreated automatically
* When performing a graceful shutdown, Dendrite will now wait for NATS Server to shutdown completely, which should avoid some corruption of data on-disk
* The `create-account` tool has seen a number of improvements, will now ask for passwords automatically

### Fixes

* The `/sync` endpoint will no longer lose state events when truncating the timeline for history visibility
* The `/context` endpoint now works correctly with `lazy_load_members`
* The `/directory/list/room/{roomID}` endpoint now correctly reports whether a room is published in the server room directory or not
* Some bugs around appservice username validation have been fixed
* Roomserver output messages are no longer unnecessarily inflated by state events, which should reduce the number of NATS message size errors
* Stream IDs for device list updates are now always 64-bit, which should fix some problems when running Dendrite on a 32-bit system
* Purging room state in the sync API has been fixed after a faulty database query was corrected
* The federation client will now release host records for remote destinations after 5 minutes instead of holding them in memory forever
* Remote media requests will now correctly return an error if the file cannot be found or downloaded
* A panic in the media API that could happen when the remote file doesn't exist has been fixed
* Various bugs around membership state and invites have been fixed
* The memberships table will now be correctly updated when rejecting a federated invite
* The client API and appservice API will now access the user database using the user API rather than accessing the database directly

## Dendrite 0.6.5 (2022-03-04)

### Features

* Early support for push notifications has been added, with support for push rules, pushers, HTTP push gateways and the `/notifications` endpoint (contributions by [danpe](https://github.com/danpe), [PiotrKozimor](https://github.com/PiotrKozimor) and [tommie](https://github.com/tommie))
* Spaces Summary (MSC2946) is now correctly supported (when `msc2946` is enabled in the config)
* All media API endpoints are now available under the `/v3` namespace
* Profile updates (display name and avatar) are now sent asynchronously so they shouldn't block the client for a very long time
* State resolution v2 has been optimised further to considerably reduce the number of memory allocations
* State resolution v2 will no longer duplicate events unnecessarily when calculating the auth difference
* The `create-account` tool now has a `-reset-password` option for resetting the passwords of existing accounts
* The `/sync` endpoint now calculates device list changes much more quickly with less RAM used
* The `/messages` endpoint now lazy-loads members correctly

### Fixes

* Read receipts now work correctly by correcting bugs in the stream positions and receipt coalescing
* Topological sorting of state and join responses has been corrected, which should help to reduce the number of auth problems when joining new federated rooms
* Media thumbnails should now work properly after having unnecessarily strict rate limiting removed
* The roomserver no longer holds transactions for as long when processing input events
* Uploading device keys and cross-signing keys will now correctly no-op if there were no changes
* Parameters are now remembered correctly during registration
* Devices can now only be deleted within the appropriate UIA flow
* The `/context` endpoint now returns 404 instead of 500 if the event was not found
* SQLite mode will no longer leak memory as a result of not closing prepared statements

## Dendrite 0.6.4 (2022-02-21)

### Features

* All Client-Server API endpoints are now available under the `/v3` namespace
* The `/whoami` response format now matches the latest Matrix spec version
* Support added for the `/context` endpoint, which should help clients to render quote-replies correctly
* Accounts now have an optional account type field, allowing admin accounts to be created
* Server notices are now supported
* Refactored the user API storage to deduplicate a significant amount of code, as well as merging both user API databases into a single database
  * The account database is now used for all user API storage and the device database is now obsolete
  * For some installations that have separate account and device databases, this may result in access tokens being revoked and client sessions being logged out — users may need to log in again
  * The above can be avoided by moving the `device_devices` table into the account database manually
* Guest registration can now be separately disabled with the new `client_api.guests_disabled` configuration option
* Outbound connections now obey proxy settings from the environment, deprecating the `federation_api.proxy_outbound` configuration options

### Fixes

* The roomserver input API will now strictly consume only one database transaction per room, which should prevent situations where the roomserver can deadlock waiting for database connections to become available
* Room joins will now fall back to federation if the local room state is insufficient to create a membership event
* Create events are now correctly filtered from federation `/send` transactions
* Excessive logging when federation is disabled should now be fixed
* Dendrite will no longer panic if trying to retire an invite event that has not been seen yet
* The device list updater will now wait for longer after a connection issue, rather than flooding the logs with errors
* The device list updater will no longer produce unnecessary output events for federated key updates with no changes, which should help to reduce CPU usage
* Local device name changes will now generate key change events correctly
* The sync API will now try to share device list update notifications even if all state key NIDs cannot be fetched
* An off-by-one error in the sync stream token handling which could result in a crash has been fixed
* State events will no longer be re-sent unnecessary by the roomserver to other components if they have already been sent, which should help to reduce the NATS message sizes on the roomserver output topic in some cases
* The roomserver input API now uses the process context and should handle graceful shutdowns better
* Guest registration is now correctly disabled when the `client_api.registration_disabled` configuration option is set
* One-time encryption keys are now cleaned up correctly when a device is logged out or removed
* Invalid state snapshots in the state storage refactoring migration are now reset rather than causing a panic at startup

## Dendrite 0.6.3 (2022-02-10)

### Features

* Initial support for `m.login.token`
* A number of regressions from earlier v0.6.x versions should now be corrected

### Fixes

* Missing state is now correctly retrieved in cases where a gap in the timeline was closed but some of those events were missing state snapshots, which should help to unstick slow or broken rooms
* Fixed a transaction issue where inserting events into the database could deadlock, which should stop rooms from getting stuck
* Fixed a problem where rejected events could result in rolled back database transactions
* Avoided a potential race condition on fetching latest events by using the room updater instead
* Processing events from `/get_missing_events` will no longer result in potential recursion
* Federation events are now correctly generated for updated self-signing keys and signed devices
* Rejected events can now be un-rejected if they are reprocessed and all of the correct conditions are met
* Fetching missing auth events will no longer error as long as all needed events for auth were satisfied
* Users can now correctly forget rooms if they were not a member of the room

## Dendrite 0.6.2 (2022-02-04)

### Fixes

* Resolves an issue where the key change consumer in the keyserver could consume extreme amounts of CPU

## Dendrite 0.6.1 (2022-02-04)

### Features

* Roomserver inputs now take place with full transactional isolation in PostgreSQL deployments
* Pull consumers are now used instead of push consumers when retrieving messages from NATS to better guarantee ordering and to reduce redelivery of duplicate messages
* Further logging tweaks, particularly when joining rooms
* Improved calculation of servers in the room, when checking for missing auth/prev events or state
* Dendrite will now skip dead servers more quickly when federating by reducing the TCP dial timeout
* The key change consumers have now been converted to use native NATS code rather than a wrapper
* Go 1.16 is now the minimum supported version for Dendrite

### Fixes

* Local clients should now be notified correctly of invites
* The roomserver input API now has more time to process events, particularly when fetching missing events or state, which should fix a number of errors from expired contexts
* Fixed a panic that could happen due to a closed channel in the roomserver input API
* Logging in with uppercase usernames from old installations is now supported again (contributed by [hoernschen](https://github.com/hoernschen))
* Federated room joins now have more time to complete and should not fail due to expired contexts
* Events that were sent to the roomserver along with a complete state snapshot are now persisted with the correct state, even if they were rejected or soft-failed

## Dendrite 0.6.0 (2022-01-28)

### Features

* NATS JetStream is now used instead of Kafka and Naffka
  * For monolith deployments, a built-in NATS Server is embedded into Dendrite or a standalone NATS Server deployment can be optionally used instead
  * For polylith deployments, a standalone NATS Server deployment is required
  * Requires the version 2 configuration file — please see the new `dendrite-config.yaml` sample config file
  * Kafka and Naffka are no longer supported as of this release
* The roomserver is now responsible for fetching missing events and state instead of the federation API
  * Removes a number of race conditions between the federation API and roomserver, which reduces duplicate work and overall lowers CPU usage
* The roomserver input API is now strictly ordered with support for asynchronous requests, smoothing out incoming federation significantly
* Consolidated the federation API, federation sender and signing key server into a single component
  * If multiple databases are used, tables for the federation sender and signing key server should be merged into the federation API database (table names have not changed)
* Device list synchronisation is now database-backed rather than using the now-removed Kafka logs

### Fixes

* The code for fetching missing events and state now correctly identifies when gaps in history have been closed, so federation traffic will consume less CPU and memory than before
* The stream position is now correctly advanced when typing notifications time out in the sync API
* Event NIDs are now correctly returned when persisting events in the roomserver in SQLite mode
  * The built-in SQLite was updated to version 3.37.0 as a result
* The `/event_auth` endpoint now strictly returns the auth chain for the requested event without loading the room state, which should reduce spikes in memory usage
* Filters are now correctly sent when using federated public room directories (contributed by [S7evinK](https://github.com/S7evinK))
* Login usernames are now squashed to lower-case (contributed by [BernardZhao](https://github.com/BernardZhao))
* The logs should no longer be flooded with `Failed to get server ACLs for room` warnings at startup
* Backfilling will now attempt federation as a last resort when trying to retrieve missing events from the database fails

## Dendrite 0.5.1 (2021-11-16)

### Features

* Experimental (although incomplete) support for joining version 8 and 9 rooms
* State resolution v2 optimisations (close to 20% speed improvement thanks to reduced allocations)
* Optimisations made to the federation `/send` endpoint which avoids duplicate work, reduces CPU usage and smooths out incoming federation
* The sync API now consumes less CPU when generating sync responses (optimised `SelectStateInRange`)
* Support for serving the `.well-known/matrix/server` endpoint from within Dendrite itself (contributed by [twentybit](https://github.com/twentybit))
* Support for thumbnailing WebP media (contributed by [hacktivista](https://github.com/hacktivista))

### Fixes

* The `/publicRooms` handler now handles `POST` requests in addition to `GET` correctly
* Only valid canonical aliases will be returned in the `/publicRooms` response
* The media API now correctly handles `max_file_size_bytes` being configured to `0` (contributed by [database64128](https://github.com/database64128))
* Unverifiable auth events in `/send_join` responses no longer result in a panic
* Build issues on Windows are now resolved (contributed by [S7evinK](https://github.com/S7evinK))
* The default power levels in a room now set the invite level to 50, as per the spec
* A panic has been fixed when malformed messages are received in the key change consumers

## Dendrite 0.5.0 (2021-08-24)

### Features

* Support for serverside key backups has been added, allowing your E2EE keys to be backed up and to be restored after logging out or when logging in from a new device
* Experimental support for cross-signing has been added, allowing verifying your own device keys and verifying other user's public keys
* Dendrite can now send logs to a TCP syslog server by using the `syslog` logger type (contributed by [sambhavsaggi](https://github.com/sambhavsaggi))
* Go 1.15 is now the minimum supported version for Dendrite

### Fixes

* Device keys are now cleaned up from the keyserver when the user API removes a device session
* The `M_ROOM_IN_USE` error code is now returned when a room alias is already taken (contributed by [nivekuil](https://github.com/nivekuil))
* A bug in the state storage migration has been fixed where room create events had incorrect state snapshots
* A bug when deactivating accounts caused by only reading the deprecated username field has been fixed

## Dendrite 0.4.1 (2021-07-26)

### Features

* Support for room version 7 has been added
* Key notary support is now more complete, allowing Dendrite to be used as a notary server for looking up signing keys
* State resolution v2 performance has been optimised further by caching the create event, power levels and join rules in memory instead of parsing them repeatedly
* The media API now handles cases where the maximum file size is configured to be less than 0 for unlimited size
* The `initial_state` in a `/createRoom` request is now respected when creating a room
* Code paths for checking if servers are joined to rooms have been optimised significantly

### Fixes

* A bug resulting in `cannot xref null state block with snapshot` during the new state storage migration has been fixed
* Invites are now retired correctly when rejecting an invite from a remote server which is no longer reachable
* The DNS cache `cache_lifetime` option is now handled correctly (contributed by [S7evinK](https://github.com/S7evinK))
* Invalid events in a room join response are now dropped correctly, rather than failing the entire join
* The `prev_state` of an event will no longer be populated incorrectly to the state of the current event
* Receiving an invite to an unsupported room version will now correctly return the `M_UNSUPPORTED_ROOM_VERSION` error code instead of `M_BAD_JSON` (contributed by [meenal06](https://github.com/meenal06))

## Dendrite 0.4.0 (2021-07-12)

### Features

* All-new state storage in the roomserver, which dramatically reduces disk space utilisation
  * State snapshots and blocks are now aggressively deduplicated and reused wherever possible, with state blocks being reduced by up to 15x and snapshot references being reduced up to 2x
  * Dendrite will upgrade to the new state storage automatically on the first run after upgrade, although this may take some time depending on the size of the state storage
* Appservice support has been improved significantly, with many bridges now working correctly with Dendrite
  * Events are now correctly sent to appservices based on room memberships
  * Aliases and namespaces are now handled correctly, calling the appservice to query for aliases as needed
  * Appservice user registrations are no longer being subject to incorrect validation checks
* Shared secret registration has now been implemented correctly
* The roomserver input API implements a new queuing system to reduce backpressure across rooms
* Checking if the local server is in a room has been optimised substantially, reducing CPU usage
* State resolution v2 has been optimised further by improving the power level checks, reducing CPU usage
* The federation API `/send` endpoint now deduplicates missing auth and prev events more aggressively to reduce memory usage
* The federation API `/send` endpoint now uses workers to reduce backpressure across rooms
* The bcrypt cost for password storage is now configurable with the `user_api.bcrypt_cost` option
* The federation API will now use significantly less memory when calling `/get_missing_events`
* MSC2946 Spaces endpoints have been updated to stable endpoint naming
* The media API can now be configured without a maximum file size
* A new `dendrite-upgrade-test` test has been added for verifying database schema upgrades across versions
* Added Prometheus metrics for roomserver backpressure, excessive device list updates and federation API event processing summaries
* Sentry support has been added for error reporting

### Fixes

* Removed the legacy `/v1` register endpoint. Dendrite only implements `/r0` of the CS API, and the legacy `/v1` endpoint had implementation errors which made it possible to bypass shared secret registration (thanks to Jakob Varmose Bentzen for reporting this)
* Attempting to register an account that already exists now returns a sensible error code rather than a HTTP 500
* Dendrite will no longer attempt to `/make_join` with itself if listed in the request `server_names`
* `/sync` will no longer return immediately if there is nothing to sync, which happened particularly with new accounts, causing high CPU usage
* Malicious media uploads can no longer exhaust all available memory (contributed by [S7evinK](https://github.com/S7evinK))
* Selecting one-time keys from the database has been optimised (contributed by [S7evinK](https://github.com/S7evinK))
* The return code when trying to fetch missing account data has been fixed (contributed by [adamgreig](https://github.com/adamgreig))
* Dendrite will no longer attempt to use `/make_leave` over federation when rejecting a local invite
* A panic has been fixed in `QueryMembershipsForRoom`
* A panic on duplicate membership events has been fixed in the federation sender
* A panic has been fixed in in `IsInterestedInRoomID` (contributed by [bodqhrohro](https://github.com/bodqhrohro))
* A panic in the roomserver has been fixed when handling empty state sets
* A panic in the federation API has been fixed when handling cached events

## Dendrite 0.3.11 (2021-03-02)

### Fixes

* **SECURITY:** A bug in SQLite mode which could cause the registration flow to complete unexpectedly for existing accounts has been fixed (PostgreSQL deployments are not affected)
* A panic in the federation sender has been fixed when shutting down destination queues
* The `/keys/upload` endpoint now correctly returns the number of one-time keys in response to an empty upload request

## Dendrite 0.3.10 (2021-02-17)

### Features

* In-memory caches will now gradually evict old entries, reducing idle memory usage
* Federation sender queues will now be fully unloaded when idle, reducing idle memory usage
* The `power_level_content_override` option is now supported in `/createRoom`
* The `/send` endpoint will now attempt more servers in the room when trying to fetch missing events or state

### Fixes

* A panic in the membership updater has been fixed
* Events in the sync API that weren't excluded from sync can no longer be incorrectly excluded from sync by backfill
* Retrieving remote media now correcly respects the locally configured maximum file size, even when the `Content-Length` header is unavailable
* The `/send` endpoint will no longer hit the database more than once to find servers in the room

## Dendrite 0.3.9 (2021-02-04)

### Features

* Performance of initial/complete syncs has been improved dramatically
* State events that can't be authed are now dropped when joining a room rather than unexpectedly causing the room join to fail
* State events that already appear in the timeline will no longer be requested from the sync API database more than once, which may reduce memory usage in some cases

### Fixes

* A crash at startup due to a conflict in the sync API account data has been fixed
* A crash at startup due to mismatched event IDs in the federation sender has been fixed
* A redundant check which may cause the roomserver memberships table to get out of sync has been removed

## Dendrite 0.3.8 (2021-01-28)

### Fixes

* A well-known lookup regression in version 0.3.7 has been fixed

## Dendrite 0.3.7 (2021-01-26)

### Features

* Sync filtering support (for event types, senders and limits)
* In-process DNS caching support for deployments where a local DNS caching resolver is not available (disabled by default)
* Experimental support for MSC2444 (Peeking over Federation) has been merged
* Experimental federation support for MSC2946 (Spaces Summary) has been merged

### Fixes

* Dendrite will no longer load a given event more than once for state resolution, which may help to reduce memory usage and database I/O slightly in some cases
* Large well-known responses will no longer use significant amounts of memory

## Dendrite 0.3.6 (2021-01-18)

### Features

* Experimental support for MSC2946 (Spaces Summary) has been merged
* Send-to-device messages have been refactored and now take advantage of having their own stream position, making delivery more reliable
* Unstable features and MSCs are now listed in `/versions` (contributed by [sumitks866](https://github.com/sumitks866))
* Well-known and DNS SRV record results for federated servers are now cached properly, improving outbound federation performance and reducing traffic

### Fixes

* Updating forward extremities will no longer result in so many unnecessary state snapshots, reducing on-going disk usage in the roomserver database
* Pagination tokens for `/messages` have been fixed, which should improve the reliability of scrollback/pagination
* Dendrite now avoids returning `null`s in fields of the `/sync` response, and omitting some fields altogether when not needed, which should fix sync issues with Element Android
* Requests for user device lists now time out quicker, which prevents federated `/send` requests from also timing out in many cases
* Empty push rules are no longer sent over and over again in `/sync`
* An integer overflow in the device list updater which could result in panics on 32-bit platforms has been fixed (contributed by [Lesterpig](https://github.com/Lesterpig))
* Event IDs are now logged properly in federation sender and sync API consumer errors

## Dendrite 0.3.5 (2021-01-11)

### Features

* All `/sync` streams are now logically separate after a refactoring exercise

### Fixes

* Event references are now deeply checked properly when calculating forward extremities, reducing the amount of forward extremities in most cases, which improves RAM utilisation and reduces the work done by state resolution
* Sync no longer sends incorrect `next_batch` tokens with old stream positions, reducing flashbacks of old messages in clients
* The federation `/send` endpoint no longer uses the request context, which could result in some events failing to be persisted if the sending server gave up the HTTP connection
* Appservices can now auth as users in their namespaces properly

## Dendrite 0.3.4 (2020-12-18)

### Features

* The stream tokens for `/sync` have been refactored, giving PDUs, typing notifications, read receipts, invites and send-to-device messages their own respective stream positions, greatly improving the correctness of sync
* A new roominfo cache has been added, which results in less database hits in the roomserver
* Prometheus metrics have been added for sync requests, destination queues and client API event send perceived latency

### Fixes

* Event IDs are no longer recalculated so often in `/sync`, which reduces CPU usage
* Sync requests are now woken up correctly for our own device list updates
* The device list stream position is no longer lost, so unnecessary device updates no longer appear in every other sync
* A crash on concurrent map read/writes has been fixed in the stream token code
* The roomserver input API no longer starts more worker goroutines than needed
* The roomserver no longer uses the request context for queued tasks which could lead to send requests failing to be processed
* A new index has been added to the sync API current state table, which improves lookup performance significantly
* The client API `/joined_rooms` endpoint no longer incorrectly returns `null` if there are 0 rooms joined
* The roomserver will now query appservices when looking up a local room alias that isn't known
* The check on registration for appservice-exclusive namespaces has been fixed

## Dendrite 0.3.3 (2020-12-09)

### Features

* Federation sender should now use considerably less CPU cycles and RAM when sending events into large rooms
* The roomserver now uses considerably less CPU cycles by not calculating event IDs so often
* Experimental support for [MSC2836](https://github.com/matrix-org/matrix-doc/pull/2836) (threading) has been merged
* Dendrite will no longer hold federation HTTP connections open unnecessarily, which should help to reduce ambient CPU/RAM usage and hold fewer long-term file descriptors

### Fixes

* A bug in the latest event updater has been fixed, which should prevent the roomserver from losing forward extremities in some rare cases
* A panic has been fixed when federation is disabled (contributed by [kraem](https://github.com/kraem))
* The response format of the `/joined_members` endpoint has been fixed (contributed by [alexkursell](https://github.com/alexkursell))

## Dendrite 0.3.2 (2020-12-02)

### Features

* Federation can now be disabled with the `global.disable_federation` configuration option

### Fixes

* The `"since"` parameter is now checked more thoroughly in the sync API, which led to a bug that could cause forgotten rooms to reappear (contributed by [kaniini](https://github.com/kaniini))
* The polylith now proxies signing key requests through the federation sender correctly
* The code for checking if remote servers are allowed to see events now no longer wastes CPU time retrieving irrelevant state events

## Dendrite 0.3.1 (2020-11-20)

### Features

* Memory optimisation by reference passing, significantly reducing the number of allocations and duplication in memory
* A hook API has been added for experimental MSCs, with an early implementation of MSC2836
* The last seen timestamp and IP address are now updated automatically when calling `/sync`
* The last seen timestamp and IP address are now reported in `/_matrix/client/r0/devices` (contributed by [alexkursell](https://github.com/alexkursell))
* An optional configuration option `sync_api.real_ip_header` has been added for specifying which HTTP header contains the real client IP address (for if Dendrite is running behind a reverse HTTP proxy)
* Partial implementation of `/_matrix/client/r0/admin/whois` (contributed by [DavidSpenler](https://github.com/DavidSpenler))

### Fixes

* A concurrency bug has been fixed in the federation API that could cause Dendrite to crash
* The error when registering a username with invalid characters has been corrected (contributed by [bodqhrohro](https://github.com/bodqhrohro))

## Dendrite 0.3.0 (2020-11-16)

### Features

* Read receipts (both inbound and outbound) are now supported (contributed by [S7evinK](https://github.com/S7evinK))
* Forgetting rooms is now supported (contributed by [S7evinK](https://github.com/S7evinK))
* The `-version` command line flag has been added (contributed by [S7evinK](https://github.com/S7evinK))

### Fixes

* User accounts that contain the `=` character can now be registered
* Backfilling should now work properly on rooms with world-readable history visibility (contributed by [MayeulC](https://github.com/MayeulC))
* The `gjson` dependency has been updated for correct JSON integer ranges
* Some more client event fields have been marked as omit-when-empty (contributed by [S7evinK](https://github.com/S7evinK))
* The `build.sh` script has been updated to work properly on all POSIX platforms (contributed by [felix](https://github.com/felix))

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
