# Dendrite [![Build Status](https://travis-ci.org/matrix-org/dendrite.svg?branch=master)](https://travis-ci.org/matrix-org/dendrite)

Dendrite will be a matrix homeserver written in go.

# Install

Dendrite is still very much a work in progress, but those wishing to work on it
may be interested in the installation instructions in [INSTALL.md](INSTALL.md).

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
the event logs because it would take too long to extract the information as the
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

## How things are supposed to work.

### Local client sends an event in an existing room.

  0) The client sends a PUT `/_matrix/client/r0/rooms/{roomId}/send` request
    and an HTTP loadbalancer routes the request to a ClientAPI.

  1) The ClientAPI:

    * Authenticates the local user using the `access_token` sent in the HTTP
      request.
    * Checks if it has already processed or is processing a request with the
      same `txnID`.
    * Calculates which state events are needed to auth the request.
    * Queries the necessary state events and the latest events in the room
      from the RoomServer.
    * Confirms that the room exists and checks whether the event is allowed by
      the auth checks.
    * Builds and signs the events.
    * Writes the event to a "InputRoomEvent" kafka topic.
    * Send a `200 OK` response to the client.

  2) The RoomServer reads the event from "InputRoomEvent" kafka topic:

    * Checks if it has already has a copy of the event.
    * Checks if the event is allowed by the auth checks using the auth events
      at the event.
    * Calculates the room state at the event.
    * Works out what the latest events in the room after processing this event
      are.
    * Calculate how the changes in the latest events affect the current state
      of the room.
    * TODO: Workout what events determine the visibility of this event to other
      users
    * Writes the event along with the changes in current state to an
      "OutputRoomEvent" kafka topic. It writes all the events for a room to
      the same kafka partition.

  3a) The ClientSync reads the event from the "OutputRoomEvent" kafka topic:

    * Updates its copy of the current state for the room.
    * Works out which users need to be notified about the event.
    * Wakes up any pending `/_matrix/client/r0/sync` requests for those users.
    * Adds the event to the recent timeline events for the room.

  3b) The FederationSender reads the event from the "OutputRoomEvent" kafka topic:

    * Updates its copy of the current state for the room.
    * Works out which remote servers need to be notified about the event.
    * Sends a `/_matrix/federation/v1/send` request to those servers.
    * Or if there is a request in progress then add the event to a queue to be
      sent when the previous request finishes.

### Remote server sends an event in an existing room.

  0) The remote server sends a `PUT /_matrix/federation/v1/send` request and an
    HTTP loadbalancer routes the request to a FederationReceiver.

  1) The FederationReceiver:

    * Authenticates the remote server using the "X-Matrix" authorisation header.
    * Checks if it has already processed or is processing a request with the
      same `txnID`.
    * Checks the signatures for the events.
      Fetches the ed25519 keys for the event senders if necessary.
    * Queries the RoomServer for a copy of the state of the room at each event.
    * If the RoomServer doesn't know the state of the room at an event then
      query the state of the room at the event from the remote server using
      `GET /_matrix/federation/v1/state_ids` falling back to
      `GET /_matrix/federation/v1/state` if necessary.
    * Once the state at each event is known check whether the events are
      allowed by the auth checks against the state at each event.
    * For each event that is allowed write the event to the "InputRoomEvent"
      kafka topic.
    * Send a 200 OK response to the remote server listing which events were
      successfully processed and which events failed

  2) The RoomServer processes the event the same as it would a local event.

  3a) The ClientSync processes the event the same as it would a local event.

# TODO

 - [ ] gomatrixlib
   - [x] Canonical JSON.
   - [x] Signed JSON.
   - [x] Event hashing.
   - [x] Event signing.
   - [x] Federation server discovery.
   - [x] Federation key lookup.
   - [ ] Federation request signing.
   - [x] Event authentication.
   - [ ] Event visibility.
   - [x] State resolution.
   - [ ] Third party invites authentication.
 - [ ] Room Server
   - [x] Inputting new events from logs.
   - [ ] Inputting back-filled events from logs.
   - [x] Outputting events and current state to logs.
   - [ ] Querying state at an event.
   - [x] Querying current forward extremities and state.
   - [ ] Querying message history.
   - [ ] Exporting/importing messages from other servers.
   - [ ] Other Room Server stuff.
 - [ ] Client Room Send
   - [x] Handling /client/r0/room/... HTTP PUTs
   - [ ] Talk to remote servers for joins to remote servers.
   - [x] Outputting new events to logs.
   - [ ] Updating the last active time in presence.
   - [ ] Other Client Room Send stuff.
 - [ ] Client Sync
   - [ ] Inputting new room events and state from the logs.
   - [ ] Handling /client/r0/sync HTTP GETs
   - [ ] Outputting whether the client is syncing to the logs.
   - [ ] Other Client Sync Stuff.
 - [ ] Other Components.
