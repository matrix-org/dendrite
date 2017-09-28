# Ping-Pong

```bash
./build/examples/ping/pong
```

This example creates a client and server channel.  The server channel registers
a `PingService` with a `ping` method, which takes request `Headers` and a `Ping` body
and returns the same `Headers` along with a `Pong` body.  The client sends a ping
request to the server.

Note that every instance is bidirectional, so the same channel can be used for
both sending and receiving requests to peers.  New connections are initiated on
demand.
