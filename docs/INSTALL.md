# Installing Dendrite

Dendrite can be run in one of two configurations:

* **Polylith mode**: A cluster of individual components, dealing with different
  aspects of the Matrix protocol (see [WIRING.md](WIRING.md)). Components communicate with each other using internal HTTP APIs and [Apache Kafka](https://kafka.apache.org). This will almost certainly be the preferred model
  for large-scale deployments.

* **Monolith mode**: All components run in the same process. In this mode,
   Kafka is completely optional and can instead be replaced with an in-process
   lightweight implementation called [Naffka](https://github.com/matrix-org/naffka). This will usually be the preferred model for low-volume, low-user
   or experimental deployments.

Regardless of whether you are running in polylith or monolith mode, each Dendrite component that requires storage has its own database. Both Postgres
and SQLite are supported and can be mixed-and-matched across components as
needed in the configuration file.

Be advised that Dendrite is still developmental and it's not recommended for
use in production environments yet!

## Requirements

Dendrite requires:

* Go 1.13 or higher
* Postgres 9.5 or higher (if using Postgres databases, not needed for SQLite)

If you want to run a polylith deployment, you also need:

* Apache Kafka 0.10.2+

## Building up a monolith deploment

Start by cloning the code:

```bash
git clone https://github.com/matrix-org/dendrite
cd dendrite
```

Then build it:

```bash
go build -o bin/dendrite-monolith-server ./cmd/dendrite-monolith-server
go build -o bin/generate-keys ./cmd/generate-keys
```

## Building up a polylith deployment

Start by cloning the code:

```bash
git clone https://github.com/matrix-org/dendrite
cd dendrite
```

Then build it:

```bash
./build.sh
```

Install and start Kafka (c.f. [scripts/install-local-kafka.sh](scripts/install-local-kafka.sh)):

```bash
KAFKA_URL=http://archive.apache.org/dist/kafka/2.1.0/kafka_2.11-2.1.0.tgz

# Only download the kafka if it isn't already downloaded.
test -f kafka.tgz || wget $KAFKA_URL -O kafka.tgz
# Unpack the kafka over the top of any existing installation
mkdir -p kafka && tar xzf kafka.tgz -C kafka --strip-components 1

# Start the zookeeper running in the background.
# By default the zookeeper listens on localhost:2181
kafka/bin/zookeeper-server-start.sh -daemon kafka/config/zookeeper.properties

# Start the kafka server running in the background.
# By default the kafka listens on localhost:9092
kafka/bin/kafka-server-start.sh -daemon kafka/config/server.properties
```

On macOS, you can use [Homebrew](https://brew.sh/) for easier setup of Kafka:

```bash
brew install kafka
brew services start zookeeper
brew services start kafka
```

## Configuration

### SQLite database setup

Dendrite can use the built-in SQLite database engine for small setups.
The SQLite databases do not need to be preconfigured - Dendrite will
create them automatically at startup.

### Postgres database setup

Assuming that Postgres 9.5 (or later) is installed:

* Create role, choosing a new password when prompted:

  ```bash
  sudo -u postgres createuser -P dendrite
  ```

* Create the component databases:

  ```bash
  for i in account device mediaapi syncapi roomserver serverkey federationsender currentstate appservice naffka; do
      sudo -u postgres createdb -O dendrite dendrite_$i
  done
  ```

(On macOS, omit `sudo -u postgres` from the above commands.)

### Server key generation

Each Dendrite server requires unique server keys.

Generate the self-signed SSL certificate for federation:

```bash
test -f server.key || openssl req -x509 -newkey rsa:4096 -keyout server.key -out server.crt -days 3650 -nodes -subj /CN=localhost
```

Generate the server signing key:

```
test -f matrix_key.pem || ./bin/generate-keys -private-key matrix_key.pem
```

### Configuration file

Create config file, based on `dendrite-config.yaml`. Call it `dendrite.yaml`. Things that will need editing include *at least*:

* The `server_name` entry to reflect the hostname of your Dendrite server
* The `database` lines with an updated connection string based on your
  desired setup, e.g. replacing `component` with the name of the component:
  * For Postgres: `postgres://dendrite:password@localhost/component`
  * For SQLite on disk: `file:component.db` or `file:///path/to/component.db`
  * Postgres and SQLite can be mixed and matched.
* The `use_naffka` option if using Naffka in a monolith deployment

There are other options which may be useful so review them all. In particular,
if you are trying to federate from your Dendrite instance into public rooms
then configuring `key_perspectives` (like `matrix.org` in the sample) can
help to improve reliability considerably by allowing your homeserver to fetch
public keys for dead homeservers from somewhere else.

## Starting a monolith server

It is possible to use Naffka as an in-process replacement to Kafka when using
the monolith server. To do this, set `use_naffka: true` in your `dendrite.yaml` configuration and uncomment the relevant Naffka line in the `database` section.
Be sure to update the database username and password if needed.

The monolith server can be started as shown below. By default it listens for
HTTP connections on port 8008, so you can configure your Matrix client to use
`http://localhost:8008` as the server. If you set `--tls-cert` and `--tls-key`
as shown below, it will also listen for HTTPS connections on port 8448.

```bash
./bin/dendrite-monolith-server --tls-cert=server.crt --tls-key=server.key
```

## Starting a polylith deployment

The following contains scripts which will run all the required processes in order to point a Matrix client at Dendrite. Conceptually, you are wiring together to form the following diagram:

```

                                         /media   +---------------------------+
                      +----------->+------------->| dendrite-media-api-server |
                      ^            ^              +---------------------------+
                      |            |            :7774
                      |            |
                      |            |
                      |            |   
                      |            |   
                      |            |   
                      |            |  
                      |            |  
                      |            |  
                      |            |     /sync    +--------------------------+                 
                      |            |   +--------->| dendrite-sync-api-server |<================++
                      |            |   |          +--------------------------+                 ||
                      |            |   |        :7773    |         ^^                          ||
Matrix      +------------------+   |   |                 |         ||    client_data           ||
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

### Client proxy

This is what Matrix clients will talk to. If you use the script below, point
your client at `http://localhost:8008`.

```bash
./bin/client-api-proxy \
--bind-address ":8008" \
--client-api-server-url "http://localhost:7771" \
--sync-api-server-url "http://localhost:7773" \
--media-api-server-url "http://localhost:7774" \
```

### Federation proxy

This is what Matrix servers will talk to. This is only required if you want
to support federation.

```bash
./bin/federation-api-proxy \
--bind-address ":8448" \
--federation-api-url "http://localhost:7772" \
--media-api-server-url "http://localhost:7774" \
```

### Client API server

This is what implements message sending. Clients talk to this via the proxy in
order to send messages.

```bash
./bin/dendrite-client-api-server --config=dendrite.yaml
```

### Room server

This is what implements the room DAG. Clients do not talk to this.

```bash
./bin/dendrite-room-server --config=dendrite.yaml
```

### Sync server

This is what implements `/sync` requests. Clients talk to this via the proxy
in order to receive messages.

```bash
./bin/dendrite-sync-api-server --config dendrite.yaml
```

### Media server

This implements `/media` requests. Clients talk to this via the proxy in
order to upload and retrieve media.

```bash
./bin/dendrite-media-api-server --config dendrite.yaml
```

### Federation API server

This implements federation requests. Servers talk to this via the proxy in
order to send transactions.  This is only required if you want to support
federation.

```bash
./bin/dendrite-federation-api-server --config dendrite.yaml
```

### Federation sender

This sends events from our users to other servers.  This is only required if
you want to support federation.

```bash
./bin/dendrite-federation-sender-server --config dendrite.yaml
```

### Appservice server

This sends events from the network to [application
services](https://matrix.org/docs/spec/application_service/unstable.html)
running locally.  This is only required if you want to support running
application services on your homeserver.

```bash
./bin/dendrite-appservice-server --config dendrite.yaml
```

### Key server

This manages end-to-end encryption keys (or rather, it will do when it's
finished).

```bash
./bin/dendrite-key-server --config dendrite.yaml
```

### User server

This manages user accounts, device access tokens and user account data,
amongst other things.

```bash
./bin/dendrite-user-api-server --config dendrite.yaml
```

