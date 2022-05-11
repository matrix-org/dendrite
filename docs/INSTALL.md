---
title: Installation
has_children: true
nav_order: 2
---



## Starting a polylith deployment

The following contains scripts which will run all the required processes in order to point a Matrix client at Dendrite.

### nginx (or other reverse proxy)

This is what your clients and federated hosts will talk to. It must forward
requests onto the correct API server based on URL:

* `/_matrix/client` to the client API server
* `/_matrix/federation` to the federation API server
* `/_matrix/key` to the federation API server
* `/_matrix/media` to the media API server

See `docs/nginx/polylith-sample.conf` for a sample configuration.

### Client API server

This is what implements CS API endpoints. Clients talk to this via the proxy in
order to send messages, create and join rooms, etc.

```bash
./bin/dendrite-polylith-multi --config=dendrite.yaml clientapi
```

### Sync server

This is what implements `/sync` requests. Clients talk to this via the proxy
in order to receive messages.

```bash
./bin/dendrite-polylith-multi --config=dendrite.yaml syncapi
```

### Media server

This implements `/media` requests. Clients talk to this via the proxy in
order to upload and retrieve media.

```bash
./bin/dendrite-polylith-multi --config=dendrite.yaml mediaapi
```

### Federation API server

This implements the federation API. Servers talk to this via the proxy in
order to send transactions.  This is only required if you want to support
federation.

```bash
./bin/dendrite-polylith-multi --config=dendrite.yaml federationapi
```

### Internal components

This refers to components that are not directly spoken to by clients. They are only
contacted by other components. This includes the following components.

#### Room server

This is what implements the room DAG. Clients do not talk to this.

```bash
./bin/dendrite-polylith-multi --config=dendrite.yaml roomserver
```

#### Appservice server

This sends events from the network to [application
services](https://matrix.org/docs/spec/application_service/unstable.html)
running locally.  This is only required if you want to support running
application services on your homeserver.

```bash
./bin/dendrite-polylith-multi --config=dendrite.yaml appservice
```

#### Key server

This manages end-to-end encryption keys for users.

```bash
./bin/dendrite-polylith-multi --config=dendrite.yaml keyserver
```

#### User server

This manages user accounts, device access tokens and user account data,
amongst other things.

```bash
./bin/dendrite-polylith-multi --config=dendrite.yaml userapi
```
