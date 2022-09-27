---
title: Configuring Dendrite
parent: Installation
nav_order: 7
permalink: /installation/configuration
---

# Configuring Dendrite

A YAML configuration file is used to configure Dendrite. Sample configuration files are
present in the top level of the Dendrite repository:

* [`dendrite-sample.monolith.yaml`](https://github.com/matrix-org/dendrite/blob/main/dendrite-sample.monolith.yaml)
* [`dendrite-sample.polylith.yaml`](https://github.com/matrix-org/dendrite/blob/main/dendrite-sample.polylith.yaml)

You will need to duplicate the sample, calling it `dendrite.yaml` for example, and then
tailor it to your installation. At a minimum, you will need to populate the following
sections:

## Server name

First of all, you will need to configure the server name of your Matrix homeserver.
This must match the domain name that you have selected whilst [configuring the domain
name delegation](domainname).

In the `global` section, set the `server_name` to your delegated domain name:

```yaml
global:
  # ...
  server_name: example.com
```

## Server signing keys

Next, you should tell Dendrite where to find your [server signing keys](signingkeys).

In the `global` section, set the `private_key` to the path to your server signing key:

```yaml
global:
  # ...
  private_key: /path/to/matrix_key.pem
```

## JetStream configuration

Monolith deployments can use the built-in NATS Server rather than running a standalone
server. If you are building a polylith deployment, or you want to use a standalone NATS
Server anyway, you can also configure that too.

### Built-in NATS Server (monolith only)

In the `global` section, under the `jetstream` key, ensure that no server addresses are
configured and set a `storage_path` to a persistent folder on the filesystem:

```yaml
global:
  # ...
  jetstream:
    in_memory: false
    storage_path: /path/to/storage/folder
    topic_prefix: Dendrite
```

### Standalone NATS Server (monolith and polylith)

To use a standalone NATS Server instance, you will need to configure `addresses` field
to point to the port that your NATS Server is listening on:

```yaml
global:
  # ...
  jetstream:
    addresses:
      - localhost:4222
    topic_prefix: Dendrite
```

You do not need to configure the `storage_path` when using a standalone NATS Server instance.
In the case that you are connecting to a multi-node NATS cluster, you can configure more than
one address in the `addresses` field.

## Database connections

Configuring database connections varies based on the [database configuration](database)
that you chose.

### Global connection pool (monolith with a single PostgreSQL database only)

If you are running a monolith deployment and want to use a single connection pool to a
single PostgreSQL database, then you must uncomment and configure the `database` section
within the `global` section:

```yaml
global:
  # ...
  database:
    connection_string: postgres://user:pass@hostname/database?sslmode=disable
    max_open_conns: 100
    max_idle_conns: 5
    conn_max_lifetime: -1
```

**You must then remove or comment out** the `database` sections from other areas of the
configuration file, e.g. under the `app_service_api`, `federation_api`, `key_server`,
`media_api`, `mscs`, `room_server`, `sync_api` and `user_api` blocks, otherwise these will
override the `global` database configuration.

### Per-component connections (all other configurations)

If you are building a polylith deployment, are using SQLite databases or separate PostgreSQL
databases per component, then you must instead configure the `database` sections under each
of the component blocks ,e.g. under the `app_service_api`, `federation_api`, `key_server`,
`media_api`, `mscs`, `room_server`, `sync_api` and `user_api` blocks.

For example, with PostgreSQL:

```yaml
room_server:
  # ...
  database:
    connection_string: postgres://user:pass@hostname/dendrite_component?sslmode=disable
    max_open_conns: 10
    max_idle_conns: 2
    conn_max_lifetime: -1
```

... or with SQLite:

```yaml
room_server:
  # ...
  database:
    connection_string: file:roomserver.db
    max_open_conns: 10
    max_idle_conns: 2
    conn_max_lifetime: -1
```

## Full-text search

Dendrite supports experimental full-text indexing using [Bleve](https://github.com/blevesearch/bleve). It is configured in the `sync_api` section as follows.

Depending on the language most likely to be used on the server, it might make sense to change the `language` used when indexing, to ensure the returned results match the expectations. A full list of possible languages can be found [here](https://github.com/blevesearch/bleve/tree/master/analysis/lang).

```yaml
sync_api:
  # ...
  search:
    enabled: false
    index_path: "./searchindex"
    language: "en"
```

## Other sections

There are other options which may be useful so review them all. In particular, if you are
trying to federate from your Dendrite instance into public rooms then configuring the
`key_perspectives` (like `matrix.org` in the sample) can help to improve reliability
considerably by allowing your homeserver to fetch public keys for dead homeservers from
another living server.
