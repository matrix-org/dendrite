---
title: Preparing database storage
parent: Installation
nav_order: 2
parent: Manual
grand_parent: Installation
permalink: /installation/manual/database
---

# Preparing database storage

Dendrite uses SQL databases to store data. Depending on the database engine being used, you
may need to perform some manual steps outlined below.

## PostgreSQL

Dendrite can automatically populate the database with the relevant tables and indexes, but
it is not capable of creating the database itself. You will need to create the database
manually.

The database **must** be created with UTF-8 encoding configured, or you will likely run into problems
with your Dendrite deployment.

You will need to create a single PostgreSQL database. Deployments
can use a single global connection pool, which makes updating the configuration file much easier.
Only one database connection string to manage and likely simpler to back up the database. All
components will be sharing the same database resources (CPU, RAM, storage).

You will most likely want to:

1. Configure a role (with a username and password) which Dendrite can use to connect to the
   database;
2. Create the database itself, ensuring that the Dendrite role has privileges over them.
   As Dendrite will create and manage the database tables, indexes and sequences by itself, the
   Dendrite role must have suitable privileges over the database.

### Connection strings

The format of connection strings for PostgreSQL databases is described in the [PostgreSQL libpq manual](https://www.postgresql.org/docs/current/libpq-connect.html#LIBPQ-CONNSTRING). Note that Dendrite only
supports the "Connection URIs" format and **will not** work with the "Keyword/Value Connection
string" format.

Example supported connection strings take the format:

* `postgresql://user:pass@hostname/database?options=...`
* `postgres://user:pass@hostname/database?options=...`

If you need to disable SSL/TLS on the database connection, you may need to append `?sslmode=disable` to the end of the connection string.

### Role creation

Create a role which Dendrite can use to connect to the database, choosing a new password when
prompted. On macOS, you may need to omit the `sudo -u postgres` from the below instructions.

```bash
sudo -u postgres createuser -P dendrite
```

### Single database creation

Create the database itself, using the `dendrite` role from above:

```bash
sudo -u postgres createdb -O dendrite -E UTF-8 dendrite
```

## SQLite

**WARNING:** The Dendrite SQLite backend is slower, less reliable and not recommended for
production usage. You should use PostgreSQL instead. We may not be able to provide support if
you run into issues with your deployment while using the SQLite backend.

SQLite deployments do not require manual database creation. Simply configure the database
filenames in the Dendrite configuration file and start Dendrite. The databases will be created
and populated automatically.

Note that Dendrite **cannot share a single SQLite database across multiple components**. Each
component must be configured with its own SQLite database filename. You will have to remove
the `global.database` section from your Dendrite config and add it to each individual section
instead in order to use SQLite.

### Connection strings

Connection strings for SQLite databases take the following forms:

* Current working directory path: `file:dendrite_component.db`
* Full specified path: `file:///path/to/dendrite_component.db`
