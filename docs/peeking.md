---
nav_exclude: true
---

## Peeking

Local peeking is implemented as per [MSC2753](https://github.com/matrix-org/matrix-doc/pull/2753).

Implementationwise, this means:

* Users call `/peek` and `/unpeek` on the clientapi from a given device.
* The clientapi delegates these via HTTP to the roomserver, which coordinates peeking in general for a given room
* The roomserver writes an NewPeek event into the kafka log headed to the syncserver
* The syncserver tracks the existence of the local peek in the syncapi_peeks table in its DB, and then starts waking up the peeking devices for the room in question, putting it in the `peek` section of the /sync response.

Peeking over federation is implemented as per [MSC2444](https://github.com/matrix-org/matrix-doc/pull/2444).

For requests to peek our rooms ("inbound peeks"):

* Remote servers call `/peek` on federationapi
  * The federationapi queries the federationsender to check if this is renewing an inbound peek or not.
  * If not, it hits the PerformInboundPeek on the roomserver to ask it for the current state of the room.
  * The roomserver atomically (in theory) adds a NewInboundPeek to its kafka stream to tell the federationserver to start peeking.
  * The federationsender receives the event, tracks the inbound peek in the federationsender_inbound_peeks table, and starts sending events to the peeking server.
  * The federationsender evicts stale inbound peeks which haven't been renewed.

For peeking into other server's rooms ("outbound peeks"):

* The `roomserver` will kick the `federationsender` much as it does for a federated `/join` in order to trigger a federated outbound `/peek`
* The `federationsender` tracks the existence of the outbound peek in in its federationsender_outbound_peeks table.
* The `federationsender` regularly renews the remote peek as long as there are still peeking devices syncing for it.
* TBD: how do we tell if there are no devices currently syncing for a given peeked room?  The syncserver needs to tell the roomserver
    somehow who then needs to warn the federationsender.
