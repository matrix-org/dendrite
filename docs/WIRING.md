# Wiring

The diagram is incomplete. The following things aren't shown on the diagram:

* Device Messages
* User Profiles
* Notification Counts
* Sending federation.
* Querying federation.
* Other things that aren't shown on the diagram.

Diagram:


    W -> Writer
    S -> Server/Store/Service/Something/Stuff
    R -> Reader

               +---+                                                    +---+                              +---+
    +----------| W |                                         +----------| S |                     +--------| R |
    |          +---+                                         | Receipts +---+                     | Client +---+
    | Federation |>=========================================>| Server     |>=====================>| Sync     |
    | Receiver   |                                           |            |                       |          |
    |            |                                 +---+     |            |                       |          |
    |            |                        +--------| W |     |            |                       |          |
    |            |                        | Client +---+     |            |                       |          |
    |            |                        | Receipt  |>=====>|            |                       |          |
    |            |                        | Updater  |       |            |                       |          |
    |            |                        +----------+       |            |                       |          |
    |            |                                           |            |                       |          |
    |            |                +---+            +---+     |            |                +---+  |          |
    |            |   +------------| W |     +------| S |     |            |       +--------| R |  |          |
    |            |   | Federation +---+     | Room +---+     |            |       | Client +---+  |          |
    |            |   | Backfill     |>=====>| Server |>=====>|            |>=====>| Push     |    |          |
    |            |   +--------------+       |        |       +------------+       |          |    |          |
    |            |                          |        |                            |          |    |          |
    |            |                          |        |>==========================>|          |    |          |
    |            |                          |        |                            +----------+    |          |
    |            |                          |        |                     +---+                  |          |
    |            |                          |        |       +-------------| R |                  |          |
    |            |                          |        |>=====>| Application +---+                  |          |
    |            |                          |        |       | Services     |                     |          |
    |            |                          |        |       +--------------+                     |          |
    |            |                          |        |                                     +---+  |          |
    |            |                          |        |                            +--------| R |  |          |
    |            |                          |        |                            | Client +---+  |          |
    |            |>========================>|        |>==========================>| Search   |    |          |
    |            |                          |        |                            |          |    |          |
    |            |                          |        |                            +----------+    |          |
    |            |                          |        |                                            |          |
    |            |                          |        |>==========================================>|          |
    |            |                          |        |                                            |          |
    |            |                +---+     |        |                  +---+                     |          |
    |            |       +--------| W |     |        |       +----------| S |                     |          |
    |            |       | Client +---+     |        |       | Presence +---+                     |          |
    |            |       | API      |>=====>|        |>=====>| Server     |>=====================>|          |
    |            |       | /send    |       +--------+       |            |                       |          |
    |            |       |          |                        |            |                       |          |
    |            |       |          |>======================>|            |<=====================<|          |
    |            |       +----------+                        |            |                       |          |
    |            |                                           |            |                       |          |
    |            |                                 +---+     |            |                       |          |
    |            |                        +--------| W |     |            |                       |          |
    |            |                        | Client +---+     |            |                       |          |
    |            |                        | Presence |>=====>|            |                       |          |
    |            |                        | Setter   |       |            |                       |          |
    |            |                        +----------+       |            |                       |          |
    |            |                                           |            |                       |          |
    |            |                                           |            |                       |          |
    |            |>=========================================>|            |                       |          |
    |            |                                           +------------+                       |          |
    |            |                                                                                |          |
    |            |                                                      +---+                     |          |
    |            |                                           +----------| S |                     |          |
    |            |                                           | EDU      +---+                     |          |
    |            |>=========================================>| Server     |>=====================>|          |
    +------------+                                           |            |                       +----------+
                                                   +---+     |            |
                                          +--------| W |     |            |
                                          | Client +---+     |            |
                                          | Typing   |>=====>|            |
                                          | Setter   |       |            |
                                          +----------+       +------------+


# Component Descriptions

Many of the components are logical rather than physical. For example it is
possible that all of the client API writers will end up being glued together
and always deployed as a single unit.

Outbound federation requests will probably need to be funnelled through a
choke-point to implement ratelimiting and backoff correctly.

## Federation Send

 * Handles `/federation/v1/send/` requests.
 * Fetches missing ``prev_events`` from the remote server if needed.
 * Fetches missing room state from the remote server if needed.
 * Checks signatures on remote events, downloading keys if needed.
 * Queries information needed to process events from the Room Server.
 * Writes room events to logs.
 * Writes presence updates to logs.
 * Writes receipt updates to logs.
 * Writes typing updates to logs.
 * Writes other updates to logs.

## Client API /send

 * Handles puts to `/client/v1/rooms/` that create room events.
 * Queries information needed to process events from the Room Server.
 * Talks to remote servers if needed for joins and invites.
 * Writes room event pdus.
 * Writes presence updates to logs.

## Client Presence Setter

 * Handles puts to the [client API presence paths](https://matrix.org/docs/spec/client_server/unstable.html#id41).
 * Writes presence updates to logs.

## Client Typing Setter

 * Handles puts to the [client API typing paths](https://matrix.org/docs/spec/client_server/unstable.html#id32).
 * Writes typing updates to logs.

## Client Receipt Updater

 * Handles puts to the [client API receipt paths](https://matrix.org/docs/spec/client_server/unstable.html#id36).
 * Writes receipt updates to logs.

## Federation Backfill

 * Backfills events from other servers
 * Writes the resulting room events to logs.
 * Is a different component from the room server itself cause it'll
   be easier if the room server component isn't making outbound HTTP requests
   to remote servers

## Room Server

 * Reads new and backfilled room events from the logs written by FS, FB and CRS.
 * Tracks the current state of the room and the state at each event.
 * Probably does auth checks on the incoming events.
 * Handles state resolution as part of working out the current state and the
   state at each event.
 * Writes updates to the current state and new events to logs.
 * Shards by room ID.

## Receipt Server

 * Reads new updates to receipts from the logs written by the FS and CRU.
 * Somehow learns enough information from the room server to workout how the
   current receipt markers move with each update.
 * Writes the new marker positions to logs
 * Shards by room ID?
 * It may be impossible to implement without folding it into the Room Server
   forever coupling the components together.

## EDU Server

 * Reads new updates to typing from the logs written by the FS and CTS.
 * Updates the current list of people typing in a room.
 * Writes the current list of people typing in a room to the logs.
 * Shards by room ID?

## Presence Server

 * Reads the current state of the rooms from the logs to track the intersection
   of room membership between users.
 * Reads updates to presence from the logs written by the FS and the CPS.
 * Reads when clients sync from the logs from the Client Sync.
 * Tracks any timers for users.
 * Writes the changes to presence state to the logs.
 * Shards by user ID somehow?

## Client Sync

 * Handle /client/v2/sync requests.
 * Reads new events and the current state of the rooms from logs written by the Room Server.
 * Reads new receipts positions from the logs written by the Receipts Server.
 * Reads changes to presence from the logs written by the Presence Server.
 * Reads changes to typing from the logs written by the EDU Server.
 * Writes when a client starts and stops syncing to the logs.

## Client Search

 * Handle whatever the client API path for event search is?
 * Reads new events and the current state of the rooms from logs writeen by the Room Server.
 * Maintains a full text search index of somekind.

## Client Push

 * Pushes unread messages to remote push servers.
 * Reads new events and the current state of the rooms from logs writeen by the Room Server.
 * Reads the position of the read marker from the Receipts Server.
 * Makes outbound HTTP hits to the push server for the client device.

## Application Service

 * Receives events from the Room Server.
 * Filters events and sends them to each registered application service.
 * Runs a separate goroutine for each application service.

# Internal Component API

Some dendrite components use internal APIs to communicate information back
and forth between each other. There are two implementations of each API, one
that uses HTTP requests and one that does not. The HTTP implementation is
used in multi-process mode, so processes on separate computers may still
communicate, whereas in single-process or Monolith mode, the direct
implementation is used. HTTP is preferred here to kafka streams as it allows
for request responses.

Running `dendrite-monolith-server` will set up direct connections between
components, whereas running each individual component (which are only run in
multi-process mode) will set up HTTP-based connections.

The functions that make HTTP requests to internal APIs of a component are
located in `/<component name>/api/<name>.go`, named according to what
functionality they cover. Each of these requests are handled in `/<component
name>/<name>/<name>.go`.

As an example, the `appservices` component allows other Dendrite components
to query external application services via its internal API. A component
would call the desired function in `/appservices/api/query.go`. In
multi-process mode, this would send an internal HTTP request, which would
be handled by a function in `/appservices/query/query.go`. In single-process
mode, no internal HTTP request occurs, instead functions are simply called
directly, thus requiring no changes on the calling component's end.
