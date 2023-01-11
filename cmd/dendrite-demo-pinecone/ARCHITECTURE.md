## Relay Server Architecture

Relay Servers function similar to the way physical mail drop boxes do. 
A node can have many associated relay servers. Matrix events can be sent to them instead of to the destination node, and the destination node will eventually retrieve them from the relay server. 
Nodes that want to send events to an offline node need to know what relay servers are associated with their intended destination. 
Currently this is manually configured in the dendrite database. In the future this information could be configurable in the app and shared automatically via other means.

Currently events are sent as complete Matrix Transactions. 
Transactions include a list of PDUs, (which contain, among other things, lists of authorization events, previous events, and signatures) a list of EDUs, and other information about the transaction. 
There is no additional information sent along with the transaction other than what is typically added to them during Matrix federation today. 
In the future this will probably need to change in order to handle more complex room state resolution during p2p usage.

### Relay Server Architecture

```
                     0                            +--------------------+
 +----------------------------------------+       |     P2P Node A     |
 |             Relay Server               |       |     +--------+     |
 |                                        |       |     | Client |     |
 |                 +--------------------+ |       |     +--------+     |
 |                 |  Relay Server API  | |       |          |         |
 |                 |                    | |       |          V         |
 |  .--------.   2 |   +-------------+  | |   1   |   +------------+   |
 | |`--------`| <----- |  Forwarder  | <------------- | Homeserver |   |
 | | Database |    |   +-------------+  | |       |   +------------+   |
 | `----------`    |                    | |       +--------------------+
 |       ^         |                    | |       
 |       |     4   |   +-------------+  | |  
 |       `------------ |  Retriever  | <------.   +--------------------+
 |                 |   +-------------+  | |   |   |     P2P Node B     |
 |                 |                    | |   |   |     +--------+     |
 |                 +--------------------+ |   |   |     | Client |     |
 |                                        |   |   |     +--------+     |
 +----------------------------------------+   |   |          |         |
                                              |   |          V         |
                                            3 |   |   +------------+   |
                                              `------ | Homeserver |   |
                                                  |   +------------+   |
                                                  +--------------------+
```

-   0: This relay server is currently only acting on behalf of `P2P Node B`. It will only receive, and later forward events that are destined for `P2P Node B`.
-   1: When `P2P Node A` fails sending directly to `P2P Node B` (after a configurable number of attempts), it checks for any known relay servers associated with `P2P Node B` and sends to all of them.
    - If sending to any of the relay servers succeeds, that transaction is considered to be successfully sent.      
-   2: The relay server `forwarder` stores the transaction json in it's database and marks it as destined for `P2P Node B`.
-   3: When `P2P Node B` comes online, it queries all it's relay servers for any missed messages.
-   4: The relay server `retriever` will look in it's database for any transactions that are destined for `P2P Node B` and returns them one at a time.

For now, it is important that we don’t design out a hybrid approach of having both sender-side and recipient-side relay servers. 
Both approaches make sense and determining which makes for a better experience depends on the use case.

#### Sender-Side Relay Servers

If we are running around truly ad-hoc, and I don't know when or where you will be able to pick up messages, then having a sender designated server makes sense to give things the best chance at making their way to the destination. 
But in order to achieve this, you are either relying on p2p presence broadcasts for the relay to know when to try forwarding (which means you are in a pretty small network), or the relay just keeps on periodically attempting to forward to the destination which will lead to a lot of extra traffic on the network.

#### Recipient-Side Relay Servers

If we have agreed to some static relay server before going off and doing other things, or if we are talking about more global p2p federation, then having a recipient designated relay server can cut down on redundant traffic since it will sit there idle until the recipient pulls events from it.
