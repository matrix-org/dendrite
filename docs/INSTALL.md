# Installing Dendrite

Dendrite can be run in one of two configurations:

* **Monolith mode**: All components run in the same process. In this mode,
   it is possible to run an in-process [NATS Server](https://github.com/nats-io/nats-server)
   instead of running a standalone deployment. This will usually be the preferred model for
   low-to-mid volume deployments, providing the best balance between performance and resource usage.

* **Polylith mode**: A cluster of individual components running in their own processes, dealing
  with different aspects of the Matrix protocol (see [WIRING.md](WIRING-Current.md)). Components
  communicate with each other using internal HTTP APIs and [NATS Server](https://github.com/nats-io/nats-server).
  This will almost certainly be the preferred model for very large deployments but scalability
  comes with a cost. API calls are expensive and therefore a polylith deployment may end up using
  disproportionately more resources for a smaller number of users compared to a monolith deployment.

In almost all cases, it is **recommended to run in monolith mode with PostgreSQL databases**.

Regardless of whether you are running in polylith or monolith mode, each Dendrite component that
requires storage has its own database connections. Both Postgres and SQLite are supported and can
be mixed-and-matched across components as needed in the configuration file.

Be advised that Dendrite is still in development and it's not recommended for
use in production environments just yet!

## Requirements

Dendrite requires:

* Go 1.16 or higher
* PostgreSQL 12 or higher (if using PostgreSQL databases, not needed for SQLite)

If you want to run a polylith deployment, you also need:

* A standalone [NATS Server](https://github.com/nats-io/nats-server) deployment with JetStream enabled

If you want to build it on Windows, you need `gcc` in the path:

* [MinGW-w64](https://www.mingw-w64.org/)

## Building Dendrite

Start by cloning the code:

```bash
git clone https://github.com/matrix-org/dendrite
cd dendrite
```

Then build it:

* Linux or UNIX-like systems:
  ```bash
  ./build.sh
  ```

* Windows:
  ```dos
  build.cmd
  ```

## Install NATS Server

Follow the [NATS Server installation instructions](https://docs.nats.io/running-a-nats-service/introduction/installation) and then [start your NATS deployment](https://docs.nats.io/running-a-nats-service/introduction/running).

JetStream must be enabled, either by passing the `-js` flag to `nats-server`,
or by specifying the `store_dir` option in the the `jetstream` configuration.

## Configuration

### PostgreSQL database setup

Assuming that PostgreSQL 12 (or later) is installed:

* Create role, choosing a new password when prompted:

  ```bash
  sudo -u postgres createuser -P dendrite
  ```

At this point you have a choice on whether to run all of the Dendrite
components from a single database, or for each component to have its
own database. For most deployments, running from a single database will
be sufficient, although you may wish to separate them if you plan to
split out the databases across multiple machines in the future.

On macOS, omit `sudo -u postgres` from the below commands.

* If you want to run all Dendrite components from a single database:

  ```bash
    sudo -u postgres createdb -O dendrite dendrite
  ```

  ... in which case your connection string will look like `postgres://user:pass@database/dendrite`.

* If you want to run each Dendrite component with its own database:

  ```bash
  for i in mediaapi syncapi roomserver federationapi appservice keyserver userapi_accounts; do
      sudo -u postgres createdb -O dendrite dendrite_$i
  done
  ```

  ... in which case your connection string will look like `postgres://user:pass@database/dendrite_componentname`.

### SQLite database setup

**WARNING:** SQLite is suitable for small experimental deployments only and should not be used in production - use PostgreSQL instead for any user-facing federating installation!

Dendrite can use the built-in SQLite database engine for small setups.
The SQLite databases do not need to be pre-built - Dendrite will
create them automatically at startup.

### Server key generation

Each Dendrite installation requires:

* A unique Matrix signing private key
* A valid and trusted TLS certificate and private key

To generate a Matrix signing private key:

```bash
./bin/generate-keys --private-key matrix_key.pem
```

**WARNING:** Make sure take a safe backup of this key! You will likely need it if you want to reinstall Dendrite, or
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
  * For Postgres: `postgres://dendrite:password@localhost/database`, e.g.
    * `postgres://dendrite:password@localhost/dendrite_userapi_account` to connect to PostgreSQL with SSL/TLS
    * `postgres://dendrite:password@localhost/dendrite_userapi_account?sslmode=disable` to connect to PostgreSQL without SSL/TLS
  * For SQLite on disk: `file:component.db` or `file:///path/to/component.db`, e.g. `file:userapi_account.db`
  * Postgres and SQLite can be mixed and matched on different components as desired.
* Either one of the following in the `jetstream` configuration section:
  * The `addresses` option — a list of one or more addresses of an external standalone
    NATS Server deployment
  * The `storage_path` — where on the filesystem the built-in NATS server should
    store durable queues, if using the built-in NATS server

There are other options which may be useful so review them all. In particular,
if you are trying to federate from your Dendrite instance into public rooms
then configuring `key_perspectives` (like `matrix.org` in the sample) can
help to improve reliability considerably by allowing your homeserver to fetch
public keys for dead homeservers from somewhere else.

**WARNING:** Dendrite supports running all components from the same database in
PostgreSQL mode, but this is **NOT** a supported configuration with SQLite. When
using SQLite, all components **MUST** use their own database file.

## Starting a monolith server

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

If the `jetstream` section of the configuration contains no `addresses` but does
contain a `store_dir`, Dendrite will start up a built-in NATS JetStream node
automatically, eliminating the need to run a separate NATS server.

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
