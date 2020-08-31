## Peeking

Peeking is implemented as per [MSC2753](https://github.com/matrix-org/matrix-doc/pull/2753).

Implementationwise, this means:
 * Users call `/peek` and `/unpeek` on the clientapi from a given device.
 * The clientapi delegates these via HTTP to the roomserver, which coordinates peeking in general for a given room
 * The roomserver writes an NewPeek event into the kafka log headed to the syncserver
 * The syncserver tracks the existence of the local peek in its DB, and then starts waking up the peeking devices for the room in question, putting it in the `peek` section of the /sync response.

Questions (given this is [my](https://github.com/ara4n) first time hacking on Dendrite):
 * The whole clientapi -> roomserver -> syncapi flow to initiate a peek seems very indirect.  Is there a reason not to just let syncapi itself host the implementation of `/peek`?

In future, peeking over federation will be added as per [MSC2444](https://github.com/matrix-org/matrix-doc/pull/2444).
 * The `roomserver` will kick the `federationsender` much as it does for a federated `/join` in order to trigger a federated `/peek`
 * The `federationsender` tracks the existence of the remote peek in question
 * The `federationsender` regularly renews the remote peek as long as there are still peeking devices syncing for it.
  * TBD: how do we tell if there are no devices currently syncing for a given peeked room?  The syncserver needs to tell the roomserver
    somehow who then needs to warn the federationsender.