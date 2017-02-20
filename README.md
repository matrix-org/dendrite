# Dendrite [![Build Status](https://travis-ci.org/matrix-org/dendrite.svg?branch=master)](https://travis-ci.org/matrix-org/dendrite) [![Coverage Status](https://coveralls.io/repos/github/matrix-org/dendrite/badge.svg?branch=master)](https://coveralls.io/github/matrix-org/dendrite?branch=master)

Dendrite will be a matrix homeserver written in go.

# Design

## Log Based Architecture

### Decomposition and Decoupling

A matrix homeserver can be built around append-only event logs built from the
messages, receipts, presence, typing notifications, device messages and other
events sent by users on the homeservers or by other homeservers.

The server would then decompose into two categories: writers that add new
entries to the logs and readers that read those entries.

The event logs then serve to decouple the two components, the writers and
readers need only agree on the format of the entries in the event log.
This format could be largely derived from the wire format of the events used
in the client and federation protocols:


     C-S API   +---------+    Event Log    +---------+   C-S API
    ---------> |         |+  (e.g. kafka)  |         |+ --------->
               | Writers || =============> | Readers ||
    ---------> |         ||                |         || --------->
     S-S API   +---------+|                +---------+|   S-S API
                +---------+                 +---------+

However the way matrix handles state events in a room creates a few
complications for this model.

 1) Writers require the room state at an event to check if it is allowed.
 2) Readers require the room state at an event to determine the users and
    servers that are allowed to see the event.
 3) A client can query the current state of the room from a reader.

The writers and readers cannot extract the necessary information directly from
the event logs because it would take to long to extract the information as the
state is built up by collecting individual state events from the event history.

The writers and readers therefore need access to something that stores copies
of the event state in a form that can be efficiently queried. One possibility
would be for the readers and writers to maintain copies of the current state
in local databases. A second possibility would be to add a dedicated component
that maintained the state of the room and exposed an API that the readers and
writers could query to get the state. The second has the advantage that the
state is calculated and stored in a single location.


     C-S API   +---------+    Log   +--------+   Log   +---------+   C-S API
    ---------> |         |+ ======> |        | ======> |         |+ --------->
               | Writers ||         |  Room  |         | Readers ||
    ---------> |         || <------ | Server | ------> |         || --------->
     S-S API   +---------+|  Query  |        |  Query  +---------+|  S-S API
                +---------+         +--------+          +---------+


The room server can annotate the events it logs to the readers with room state
so that the readers can avoid querying the room server unnecessarily.

[This architecture can be extended to cover most of the APIs.](WIRING.md)

# TODO

 - [ ] gomatrixlib
   - [x] Canonical JSON.
   - [x] Signed JSON.
   - [ ] Event hashing.
   - [ ] Event signing.
   - [x] Federation server discovery.
   - [x] Federation key lookup.
   - [ ] Federation request signing.
   - [x] Event authentication.
   - [ ] Event visibility.
   - [ ] State resolution.
   - [ ] Third party invites authentication.
 - [ ] Room Server
   - [ ] Inputing new events from logs.
   - [ ] Inputing backfilled events from logs.
   - [ ] Outputing events and current state to logs.
   - [ ] Querying state at an event.
   - [ ] Querying current forward extremities and state.
   - [ ] Querying message history.
   - [ ] Exporting/importing messages from other servers.
   - [ ] Other Room Server stuff.
 - [ ] Client Room Send
   - [ ] Handling /client/r0/room/... HTTP PUTs
   - [ ] Talk to remote servers for joins to remote servers.
   - [ ] Outputting new events to logs.
   - [ ] Updating the last active time in presence.
   - [ ] Other Client Room Send stuff.
 - [ ] Client Sync
   - [ ] Inputing new room events and state from the logs.
   - [ ] Handling /client/r0/sync HTTP GETs
   - [ ] Outputing whether the client is syncing to the logs.
   - [ ] Other Client Sync Stuff.
 - [ ] Other Components.
