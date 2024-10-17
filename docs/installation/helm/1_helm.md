---
title: Installation
parent: Helm
grand_parent: Installation
has_toc: true
nav_order: 1
permalink: /installation/helm/install
---

# Installing Dendrite using Helm

To install Dendrite using the Helm chart, you first have to add the repository using the following commands:

```bash
helm repo add dendrite https://matrix-org.github.io/dendrite/
helm repo update
```

Next you'll need to create a `values.yaml` file and configure it to your liking. All possible values can be found
[here](https://github.com/element-hq/dendrite/blob/main/helm/dendrite/values.yaml), but at least you need to configure
a `server_name`, otherwise the chart will complain about it:

```yaml
dendrite_config:
  global:
    server_name: "localhost"
```

If you are going to use an existing Postgres database, you'll also need to configure this connection:

```yaml
dendrite_config:
  global:
    database:
      connection_string: "postgresql://PostgresUser:PostgresPassword@PostgresHostName/DendriteDatabaseName"
      max_open_conns: 90
      max_idle_conns: 5
      conn_max_lifetime: -1
```

## Installing with PostgreSQL

The chart comes with a dependency on Postgres, which can be installed alongside Dendrite, this needs to be enabled in
the `values.yaml`:

```yaml
postgresql:
  enabled: true # this installs Postgres
  primary:
    persistence:
      size: 1Gi # defines the size for $PGDATA

dendrite_config:
  global:
    server_name: "localhost"
```

Using this option, the `database.connection_string` will be set for you automatically.
