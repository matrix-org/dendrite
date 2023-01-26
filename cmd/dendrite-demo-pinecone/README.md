# Pinecone Demo

This is the Dendrite Pinecone demo! It's easy to get started.

To run the homeserver, start at the root of the Dendrite repository and run:

```
go run ./cmd/dendrite-demo-pinecone
```

To connect to the static Pinecone peer used by the mobile demos run:

```
go run ./cmd/dendrite-demo-pinecone -peer wss://pinecone.matrix.org/public
```

The following command line arguments are accepted:

* `-peer tcp://a.b.c.d:e` to specify a static Pinecone peer to connect to - you will need to supply this if you do not have another Pinecone node on your network
* `-port 12345` to specify a port to listen on for client connections

Then point your favourite Matrix client to  the homeserver URL`http://localhost:8008` (or whichever `-port` you specified), create an account and log in.

If your peering connection is operational then you should see a `Connected TCP:` line in the log output. If not then try a different peer.

Once logged in, you should be able to open the room directory or join a room by its ID.

## Store & Forward Relays

To test out the store & forward relay functionality, you need a minimum of 3 instances. 
One instance will act as the relay, and the other two instances will be the users trying to communicate.
Then you can send messages between the two nodes and watch as the relay is used if the receiving node is offline.

### Launching the Nodes

Relay Server:
```
go run cmd/dendrite-demo-pinecone/main.go -dir relay/ -listen "[::]:49000"
```

Node 1:
```
go run cmd/dendrite-demo-pinecone/main.go -dir node-1/ -peer "[::]:49000" -port 8007
```

Node 2:
```
go run cmd/dendrite-demo-pinecone/main.go -dir node-2/ -peer "[::]:49000" -port 8009
```

### Database Setup

At the moment, the database must be manually configured.
For both `Node 1` and `Node 2` add the following entries to their respective `relay_server` table in the federationapi database:
```
server_name: {node_1_public_key}, relay_server_name: {relay_public_key}
server_name: {node_2_public_key}, relay_server_name: {relay_public_key}
```

After editing the database you will need to relaunch the nodes for the changes to be picked up by dendrite.

### Testing

Now you can run two separate instances of element and connect them to `Node 1` and `Node 2`.
You can shutdown one of the nodes and continue sending messages. If you wait long enough, the message will be sent to the relay server. (you can see this in the log output of the relay server) 
