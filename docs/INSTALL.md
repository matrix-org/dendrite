# Installing Dendrite

Dendrite can be run in one of two configurations:

* **Polylith mode**: A cluster of individual components, dealing with different
  aspects of the Matrix protocol (see [WIRING.md](WIRING-Current.md)). Components communicate
  with each other using internal HTTP APIs and [Apache Kafka](https://kafka.apache.org).
  This will almost certainly be the preferred model for large-scale deployments.

* **Monolith mode**: All components run in the same process. In this mode,
   Kafka is completely optional and can instead be replaced with an in-process
   lightweight implementation called [Naffka](https://github.com/matrix-org/naffka). This
   will usually be the preferred model for low-volume, low-user or experimental deployments.

For most deployments, it is **recommended to run in monolith mode with PostgreSQL databases**.

Regardless of whether you are running in polylith or monolith mode, each Dendrite component that
requires storage has its own database. Both Postgres and SQLite are supported and can be
mixed-and-matched across components as needed in the configuration file.

Be advised that Dendrite is still in development and it's not recommended for
use in production environments just yet!

## Requirements

Dendrite requires:

* Go 1.13 or higher
* Postgres 9.6 or higher (if using Postgres databases, not needed for SQLite)

If you want to run a polylith deployment, you also need:

* Apache Kafka 0.10.2+

Please note that Kafka is **not required** for a monolith deployment.

## Building Dendrite

Start by cloning the code:

```bash
git clone https://github.com/matrix-org/dendrite
cd dendrite
```

Then build it:

```bash
./build.sh
```

## Install Kafka (polylith only)

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
The SQLite databases do not need to be pre-built - Dendrite will
create them automatically at startup.

### PostgreSQL database setup

Assuming that PostgreSQL 9.6 (or later) is installed:

* Create role, choosing a new password when prompted:

  ```bash
  sudo -u postgres createuser -P dendrite
  ```

* Create the component databases:

  ```bash
  for i in mediaapi syncapi roomserver signingkeyserver federationsender appservice keyserver userapi_account userapi_device naffka; do
      sudo -u postgres createdb -O dendrite dendrite_$i
  done
  ```

(On macOS, omit `sudo -u postgres` from the above commands.)

### Server key generation

Each Dendrite installation requires:

- A unique Matrix signing private key
- A valid and trusted TLS certificate and private key

To generate a Matrix signing private key:

```bash
./bin/generate-keys --private-key matrix_key.pem
```

**Warning:** Make sure take a safe backup of this key! You will likely need it if you want to reinstall Dendrite, or
any other Matrix homeserver, on the same domain name in the future. If you lose this key, you may have trouble joining
federated rooms.

For testing, you can generate a self-signed certificate and key, although this will not work for public federation:

```bash
./bin/generate-keys --tls-cert server.crt --tls-key server.key
```

If you have server keys from an older Synapse instance, 
[convert them](serverkeyformat.md#converting-synapse-keys) to Dendrite's PEM 
format and configure them as `old_private_keys` in your config.

### Configuration file

Create config file, based on `dendrite-config.yaml`. Call it `dendrite.yaml`. Things that will need editing include *at least*:

* The `server_name` entry to reflect the hostname of your Dendrite server
* The `database` lines with an updated connection string based on your
  desired setup, e.g. replacing `database` with the name of the database:
  * For Postgres: `postgres://dendrite:password@localhost/database`, e.g. `postgres://dendrite:password@localhost/dendrite_userapi_account.db`
  * For SQLite on disk: `file:component.db` or `file:///path/to/component.db`, e.g. `file:userapi_account.db`
  * Postgres and SQLite can be mixed and matched on different components as desired.
* The `use_naffka` option if using Naffka in a monolith deployment

There are other options which may be useful so review them all. In particular,
if you are trying to federate from your Dendrite instance into public rooms
then configuring `key_perspectives` (like `matrix.org` in the sample) can
help to improve reliability considerably by allowing your homeserver to fetch
public keys for dead homeservers from somewhere else.

**WARNING:** Dendrite supports running all components from the same database in
PostgreSQL mode, but this is **NOT** a supported configuration with SQLite. When
using SQLite, all components **MUST** use their own database file.

## Starting a monolith server

It is possible to use Naffka as an in-process replacement to Kafka when using
the monolith server. To do this, set `use_naffka: true` in your `dendrite.yaml`
configuration and uncomment the relevant Naffka line in the `database` section.
Be sure to update the database username and password if needed.

The monolith server can be started as shown below. By default it listens for
HTTP connections on port 8008, so you can configure your Matrix client to use
`http://servername:8008` as the server:

```bash
./bin/dendrite-monolith-server
```

If you set `--tls-cert` and `--tls-key` as shown below, it will also listen
for HTTPS connections on port 8448:

```bash
./bin/dendrite-monolith-server --tls-cert=server.crt --tls-key=server.key
```

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

#### Federation sender

This sends events from our users to other servers.  This is only required if
you want to support federation.

```bash
./bin/dendrite-polylith-multi --config=dendrite.yaml federationsender
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

#### Signing key server

This manages signing keys for servers.

```bash
./bin/dendrite-polylith-multi --config=dendrite.yaml signingkeyserver
```

#### EDU server

This manages processing EDUs such as typing, send-to-device events and presence. Clients do not talk to

```bash
./bin/dendrite-polylith-multi --config=dendrite.yaml eduserver
```

#### User server

This manages user accounts, device access tokens and user account data,
amongst other things.

```bash
./bin/dendrite-polylith-multi --config=dendrite.yaml userapi
```

