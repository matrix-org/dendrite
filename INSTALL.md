# Installing Dendrite

Dendrite can be run in one of two configurations:

 * A cluster of individual components, dealing with different aspects of the
   Matrix protocol (see [WIRING.md](./WIRING.md)). Components communicate with
   one another via [Apache Kafka](https://kafka.apache.org).

 * A monolith server, in which all components run in the same process. In this
   configuration, Kafka can be replaced with an in-process implementation
   called [naffka](https://github.com/matrix-org/naffka).

## Requirements

 - Go 1.8+
 - Postgres 9.5+
 - For Kafka (optional if using the monolith server):
   - Unix-based system (https://kafka.apache.org/documentation/#os)
   - JDK 1.8+ / OpenJDK 1.8+
   - Apache Kafka 0.10.2+ (see [scripts/install-local-kafka.sh](scripts/install-local-kafka.sh) for up-to-date version numbers)


## Setting up a development environment

Assumes Go 1.8 and JDK 1.8 are already installed and are on PATH.

```bash
# Get the code
git clone https://github.com/matrix-org/dendrite
cd dendrite

# Build it
go get github.com/constabulary/gb/...
gb build
```

If using Kafka, install and start it (c.f. [scripts/install-local-kafka.sh](scripts/install-local-kafka.sh)):
```bash
MIRROR=http://apache.mirror.anlx.net/kafka/0.10.2.0/kafka_2.11-0.10.2.0.tgz

# Only download the kafka if it isn't already downloaded.
test -f kafka.tgz || wget $MIRROR -O kafka.tgz
# Unpack the kafka over the top of any existing installation
mkdir -p kafka && tar xzf kafka.tgz -C kafka --strip-components 1

# Start the zookeeper running in the background.
# By default the zookeeper listens on localhost:2181
kafka/bin/zookeeper-server-start.sh -daemon kafka/config/zookeeper.properties

# Start the kafka server running in the background.
# By default the kafka listens on localhost:9092
kafka/bin/kafka-server-start.sh -daemon kafka/config/server.properties
```

On MacOS, you can use [homebrew](https://brew.sh/) for easier setup of kafka

```bash
brew install kafka
brew services start zookeeper
brew services start kafka
```

## Configuration

### Postgres database setup

Dendrite requires a postgres database engine, version 9.5 or later.

* Create role:
  ```bash
  sudo -u postgres createuser -P dendrite     # prompts for password
  ```
* Create databases:
  ```bash
  for i in account device mediaapi syncapi roomserver serverkey federationsender publicroomsapi appservice naffka; do
      sudo -u postgres createdb -O dendrite dendrite_$i
  done
  ```

(On macOS, omit `sudo -u postgres` from the above commands.)

### Crypto key generation

Generate the keys:

```bash
# Generate a self-signed SSL cert for federation:
test -f server.key || openssl req -x509 -newkey rsa:4096 -keyout server.key -out server.crt -days 3650 -nodes -subj /CN=localhost

# generate ed25519 signing key
test -f matrix_key.pem || ./bin/generate-keys -private-key matrix_key.pem
```

### Configuration

Create config file, based on `dendrite-config.yaml`. Call it `dendrite.yaml`. Things that will need editing include *at least*:
* `server_name`
* `database/*`


## Starting a monolith server

It is possible to use 'naffka' as an in-process replacement to Kafka when using
the monolith server. To do this, set `use_naffka: true` in `dendrite.yaml`.

The monolith server can be started as shown below. By default it listens for
HTTP connections on port 8008, so point your client at
`http://localhost:8008`. If you set `--tls-cert` and `--tls-key` as shown
below, it will also listen for HTTPS connections on port 8448.

```bash
./bin/dendrite-monolith-server --tls-cert=server.crt --tls-key=server.key
```

## Starting a multiprocess server

The following contains scripts which will run all the required processes in order to point a Matrix client at Dendrite. Conceptually, you are wiring together to form the following diagram:

```

                                         /media   +---------------------------+
                      +----------->+------------->| dendrite-media-api-server |
                      ^            ^              +---------------------------+
                      |            |            :7774
                      |            |
                      |            |
                      |            |   /directory +----------------------------------+
                      |            |   +--------->| dendrite-public-rooms-api-server |<========++
                      |            |   |          +----------------------------------+         ||
                      |            |   |        :7775    |                                     ||
                      |            |   |    +<-----------+                                     ||
                      |            |   |    |                                                  ||
                      |            |   | /sync    +--------------------------+                 ||
                      |            |   +--------->| dendrite-sync-api-server |<================++
                      |            |   |    |     +--------------------------+                 ||
                      |            |   |    |   :7773    |         ^^                          ||
Matrix      +------------------+   |   |    |            |         ||    client_data           ||
Clients --->| client-api-proxy |-------+    +<-----------+         ++=============++           ||
            +------------------+   |   |    |                                     ||           ||
          :8008                    |   | CS API   +----------------------------+  ||           ||
                                   |   +--------->| dendrite-client-api-server |==++           ||
                                   |        |     +----------------------------+               ||
                                   |        |   :7771    |                                     ||
                                   |        |            |                                     ||
                                   |        +<-----------+                                     ||
                                   |        |                                                  ||
                                   |        |                                                  ||
                                   |        |           +----------------------+    room_event ||
                                   |        +---------->| dendrite-room-server |===============++
                                   |        |           +----------------------+               ||
                                   |        |         :7770                                    ||
                                   |        |                      ++==========================++
                                   |        +<------------+        ||
                                   |        |             |        VV
                                   |        |     +-----------------------------------+              Matrix
                                   |        |     | dendrite-federation-sender-server |------------> Servers
                                   |        |     +-----------------------------------+
                                   |        |   :7776
                                   |        |
                       +---------->+        +<-----------+
                       |                                 |
Matrix      +----------------------+  SS API  +--------------------------------+
Servers --->| federation-api-proxy |--------->| dendrite-federation-api-server |
            +----------------------+          +--------------------------------+
          :8448                             :7772


   A --> B  = HTTP requests (A = client, B = server)
   A ==> B  = Kafka (A = producer, B = consumer)
```

### Run a client api proxy

This is what Matrix clients will talk to. If you use the script below, point your client at `http://localhost:8008`.

```bash
./bin/client-api-proxy \
--bind-address ":8008" \
--client-api-server-url "http://localhost:7771" \
--sync-api-server-url "http://localhost:7773" \
--media-api-server-url "http://localhost:7774" \
--public-rooms-api-server-url "http://localhost:7775" \
```

### Run a client api

This is what implements message sending. Clients talk to this via the proxy in order to send messages.

```bash
./bin/dendrite-client-api-server --config=dendrite.yaml
```

(If this fails with `pq: syntax error at or near "ON"`, check you are using at least postgres 9.5.)

### Run a room server

This is what implements the room DAG. Clients do not talk to this.

```bash
./bin/dendrite-room-server --config=dendrite.yaml
```

### Run a sync server

This is what implements `/sync` requests. Clients talk to this via the proxy in order to receive messages.

```bash
./bin/dendrite-sync-api-server --config dendrite.yaml
```

### Run a media server

This implements `/media` requests. Clients talk to this via the proxy in order to upload and retrieve media.

```bash
./bin/dendrite-media-api-server --config dendrite.yaml
```

### Run public room server

This implements `/directory` requests. Clients talk to this via the proxy in order to retrieve room directory listings.

```bash
./bin/dendrite-public-rooms-api-server --config dendrite.yaml
```

### Run a federation api proxy

This is what Matrix servers will talk to. This is only required if you want to support federation.

```bash
./bin/federation-api-proxy \
--bind-address ":8448" \
--federation-api-url "http://localhost:7772" \
--media-api-server-url "http://localhost:7774" \
```

### Run a federation api server

This implements federation requests. Servers talk to this via the proxy in
order to send transactions.  This is only required if you want to support
federation.

```bash
./bin/dendrite-federation-api-server --config dendrite.yaml
```

### Run a federation sender server

This sends events from our users to other servers.  This is only required if
you want to support federation.

```bash
./bin/dendrite-federation-sender-server --config dendrite.yaml
```

### Run an appservice server 

This sends events from the network to [application
services](https://matrix.org/docs/spec/application_service/unstable.html)
running locally.  This is only required if you want to support running
application services on your homeserver.

```bash
./bin/dendrite-appservice-server --config dendrite.yaml
```
