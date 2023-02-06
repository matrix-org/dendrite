## Relay Server Architecture

Relay Servers function similar to the way physical mail drop boxes do. 
A node can have many associated relay servers. Matrix events can be sent to them instead of to the destination node, and the destination node will eventually retrieve them from the relay server. 
Nodes that want to send events to an offline node need to know what relay servers are associated with their intended destination. 
Currently this is manually configured in the dendrite database. In the future this information could be configurable in the app and shared automatically via other means.

Currently events are sent as complete Matrix Transactions. 
Transactions include a list of PDUs, (which contain, among other things, lists of authorization events, previous events, and signatures) a list of EDUs, and other information about the transaction. 
There is no additional information sent along with the transaction other than what is typically added to them during Matrix federation today. 
In the future this will probably need to change in order to handle more complex room state resolution during p2p usage.

### Design

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
-   2: The relay server `forwarder` stores the transaction json in its database and marks it as destined for `P2P Node B`.
-   3: When `P2P Node B` comes online, it queries all its relay servers for any missed messages.
-   4: The relay server `retriever` will look in its database for any transactions that are destined for `P2P Node B` and returns them one at a time.

For now, it is important that we don’t design out a hybrid approach of having both sender-side and recipient-side relay servers. 
Both approaches make sense and determining which makes for a better experience depends on the use case.

#### Sender-Side Relay Servers

If we are running around truly ad-hoc, and I don't know when or where you will be able to pick up messages, then having a sender designated server makes sense to give things the best chance at making their way to the destination. 
But in order to achieve this, you are either relying on p2p presence broadcasts for the relay to know when to try forwarding (which means you are in a pretty small network), or the relay just keeps on periodically attempting to forward to the destination which will lead to a lot of extra traffic on the network.

#### Recipient-Side Relay Servers

If we have agreed to some static relay server before going off and doing other things, or if we are talking about more global p2p federation, then having a recipient designated relay server can cut down on redundant traffic since it will sit there idle until the recipient pulls events from it.

### API

Relay servers make use of 2 new matrix federation endpoints.
These are:
- PUT /_matrix/federation/v1/send_relay/{txnID}/{userID}
- GET /_matrix/federation/v1/relay_txn/{userID}

#### Send_Relay

The `send_relay` endpoint is used to send events to a relay server that are destined for some other node. Servers can send events to this endpoint if they wish for the relay server to store & forward events for them when they go offline.

##### Request

###### Request Parameters

|  Name  |  Type  |  Description                                        |
|--------|--------|-----------------------------------------------------|
| txnID  | string | **Required:** The transaction ID.                   |
| userID | string | **Required:** The destination for this transaciton. |

###### Request Body

|  Name  |  Type  |  Description                           |
|--------|--------|----------------------------------------|
| pdus   | [PDU]  | **Required:** List of pdus. Max 50.    |
| edus   | [EDU]  | List of edus. May be omitted. Max 100. |

##### Responses

|  Code  |  Reason                                          |
|--------|--------------------------------------------------|
| 200    | Successfully stored transaction for forwarding.  |
| 400    | Invalid userID.                                  |
| 400    | Invalid request body.                            |
| 400    | Too many pdus or edus.                           |
| 500    | Server failed processing transaction.            |

#### Relay_Txn

The `relay_txn` endpoint is used to get events from a relay server that are destined for you. Servers can send events to this endpoint if they wish for the relay server to store & forward events for them when they go offline.

##### Request

**This needs to be changed to prevent nodes from obtaining transactions not destined for them. Possibly by adding a signature field to the request.**

###### Request Parameters

|  Name  |  Type  |  Description                                                   |
|--------|--------|----------------------------------------------------------------|
| userID | string | **Required:** The user ID that events are being requested for. |

###### Request Body

|  Name    |  Type  |  Description                           |
|----------|--------|----------------------------------------|
| entry_id | int64  | **Required:** The id of the previous transaction received from the relay. Provided in the previous response to this endpoint. |

##### Responses

|  Code  |  Reason                                          |
|--------|--------------------------------------------------|
| 200    | Successfully stored transaction for forwarding.  |
| 400    | Invalid userID.                                  |
| 400    | Invalid request body.                            |
| 400    | Invalid previous entry. Must be >= 0             |
| 500    | Server failed processing transaction.            |

###### 200 Response Body

|  Name          |  Type        |  Description                                                             |
|----------------|--------------|--------------------------------------------------------------------------|
| transaction    | Transaction  | **Required:** A matrix transaction.                                      |
| entry_id       | int64        | An ID associated with this transaction.                                  |
| entries_queued | bool         | **Required:** Whether or not there are more events stored for this user. |
