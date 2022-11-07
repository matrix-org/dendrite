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
