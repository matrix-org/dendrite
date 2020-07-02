This document details how various components communicate with each other. There are two kinds of components:
 - Public-facing: exposes CS/SS API endpoints and need to be routed to via client-api-proxy or equivalent.
 - Internal-only: exposes internal APIs and produces Kafka events.

## Internal HTTP APIs

Not everything can be done using Kafka logs. For example, requesting the latest events in a room is much better suited to
a request/response model like HTTP or RPC. Therefore, components can expose "internal APIs" which sit outside of Kafka logs.
Note in Monolith mode these are actually direct function calls and are not serialised HTTP requests.

```
   Tier 1            Sync                         FederationAPI      ClientAPI    MediaAPI
Public Facing        |                             | | |    | |        | | | |
                     2   .-------3-----------------` | |    | `--------|-|-|-|--11--------------------.
                     |   | .--------4----------------------------------` | | |                        |
                     |   | |         .---5-----------` |    |            | | |                        |
                     |   | |         |  .---6----------------------------` | |                        |
                     |   | |         |  |              |  .-----7----------` |                        |
                     |   | |         |  |              8  | |                10                       |
                     |   | |         |  |              |  | `---9----.       |                        |
                     V   V V         V  V              V  V          V       V                        V
   Tier 2           Roomserver     EDUServer         FedSender       AppService    KeyServer   ServerKeyAPI
Internal only               |                               `------------------------12----------^   ^
                            `------------------------------------------------------------13----------`

 Client ---> Server
```
- 2 (Sync -> Roomserver): When making backfill requests
- 3 (FedAPI -> Roomserver): Calculating (prev/auth events) and sending new events, processing backfill/state/state_ids requests
- 4 (ClientAPI -> Roomserver): Calculating (prev/auth events) and sending new events, processing /state requests
- 5 (FedAPI -> EDUServer): Sending typing/send-to-device events
- 6 (ClientAPI -> EDUServer): Sending typing/send-to-device events
- 7 (ClientAPI -> FedSender): Handling directory lookups
- 8 (FedAPI -> FedSender): Resetting backoffs when receiving traffic from a server. Querying joined hosts when handling alias lookup requests
- 9 (FedAPI -> AppService): Working out if the client is an appservice user
- 10 (ClientAPI -> AppService): Working out if the client is an appservice user
- 11 (FedAPI -> ServerKeyAPI): Verifying incoming event signatures
- 12 (FedSender -> ServerKeyAPI): Verifying event signatures of responses (e.g from send_join)
- 13 (Roomserver -> ServerKeyAPI): Verifying event signatures of backfilled events

In addition to this, all public facing components (Tier 1) talk to the `UserAPI` to verify access tokens and extract profile information where needed.

## Kafka logs

```
                       .----1--------------------------------------------.
                       V                                                 |
   Tier 1            Sync                         FederationAPI      ClientAPI    MediaAPI
Public Facing        ^   ^                                             ^  
                     |   |                                             |
                     2   |                                             |
                     |   `-3------------.                              |
                     |                  |                              |
                     |                  |                              |
                     |                  |                              |
                     |   .--------4-----|------------------------------`             
                     |   |              |           
   Tier 2           Roomserver     EDUServer         FedSender       AppService    KeyServer   ServerKeyAPI
Internal only              |          |                ^ ^
                           |          `-----5----------` |
                           `--------------------6--------`


Producer ----> Consumer
```
- 1 (ClientAPI -> Sync): For tracking account data
- 2 (Roomserver -> Sync): For all data to send to clients
- 3 (EDUServer -> Sync): For typing/send-to-device data to send to clients
- 4 (Roomserver -> ClientAPI): For tracking memberships for profile updates.
- 5 (EDUServer -> FedSender): For sending EDUs over federation
- 6 (Roomserver -> FedSender): For sending PDUs over federation, for tracking joined hosts.
