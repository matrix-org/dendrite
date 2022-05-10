---
title: Monolith
parent: Installation
has_toc: true
---

# Monolith Installation

## Requirements

In order to build a polylith deployment, you will need to install:

* Go 1.16 or later
* PostgreSQL 12 or later

## Build Dendrite

On UNIX systems, the `build.sh` script will build all variants of Dendrite.

```bash
./build.sh
```

The `bin` directory will contain the built binaries.

## PostgreSQL setup

First of all, you will need to create a PostgreSQL user that Dendrite can use
to connect to the database.

  ```bash
  sudo -u postgres createuser -P dendrite
  ```

At this point you have a choice on whether to run all of the Dendrite
components from a single database, or for each component to have its
own database. For most deployments, running from a single database will
be sufficient, although you may wish to separate them if you plan to
split out the databases across multiple machines in the future.

On macOS, omit `sudo -u postgres` from the below commands.

### Single database

```bash
sudo -u postgres createdb -O dendrite dendrite
```

... in which case your connection string will look like `postgres://user:pass@database/dendrite`.

### Separate databases

```bash
for i in mediaapi syncapi roomserver federationapi appservice keyserver userapi_accounts; do
    sudo -u postgres createdb -O dendrite dendrite_$i
done
```

... in which case your connection string will look like `postgres://user:pass@database/dendrite_componentname`.

## Configuration file
