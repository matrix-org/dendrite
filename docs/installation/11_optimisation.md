---
title: Optimise your installation
parent: Installation
has_toc: true
nav_order: 11
permalink: /installation/start/optimisation
---

# Optimise your installation

Now that you have Dendrite running, the following tweaks will improve the reliability
and performance of your installation.

## PostgreSQL connection limit

A PostgreSQL database engine is configured to allow only a certain number of connections.
This is typically controlled by the `max_connections` and `superuser_reserved_connections`
configuration items in `postgresql.conf`. Once these limits are violated, **PostgreSQL will
immediately stop accepting new connections** until some of the existing connections are closed.
This is a common source of misconfiguration and requires particular care.

If your PostgreSQL `max_connections` is set to `100` and `superuser_reserved_connections` is
set to `3` then you have an effective connection limit of 97 database connections. It is
therefore important to ensure that Dendrite doesn't violate that limit, otherwise database
queries will unexpectedly fail and this will cause problems both within Dendrite and for users.

If you are also running other software that uses the same PostgreSQL database engine, then you
must also take into account that some connections will be already used by your other software
and therefore will not be available to Dendrite. Check the configuration of any other software
using the same database engine for their configured connection limits and adjust your calculations
accordingly.

Dendrite has a `max_open_conns` configuration item in each `database` block to control how many
connections it will open to the database.

**If you are using the `global` database pool** then you only need to configure the
`max_open_conns` setting once in the `global` section.

**If you are defining a `database` config per component** then you will need to ensure that
the **sum total** of all configured `max_open_conns` to a given database server do not exceed
the connection limit. If you configure a total that adds up to more connections than are available
then this will cause database queries to fail.

You may wish to raise the `max_connections` limit on your PostgreSQL server to accommodate
additional connections, in which case you should also update the `max_open_conns` in your
Dendrite configuration accordingly. However be aware that this is only advisable on particularly
powerful servers that can handle the concurrent load of additional queries running at one time.

## File descriptor limit

Most platforms have a limit on how many file descriptors a single process can open. All
connections made by Dendrite consume file descriptors — this includes database connections
and network requests to remote homeservers. When participating in large federated rooms
where Dendrite must talk to many remote servers, it is often very easy to exhaust default
limits which are quite low.

We currently recommend setting the file descriptor limit to 65535 to avoid such
issues. Dendrite will log immediately after startup if the file descriptor limit is too low:

```
level=warning msg="IMPORTANT: Process file descriptor limit is currently 1024, it is recommended to raise the limit for Dendrite to at least 65535 to avoid issues"
```

UNIX systems have two limits: a hard limit and a soft limit. You can view the soft limit
by running `ulimit -Sn` and the hard limit with `ulimit -Hn`:

```bash
$ ulimit -Hn
1048576

$ ulimit -Sn
1024
```

Increase the soft limit before starting Dendrite:

```bash
ulimit -Sn 65535
```

The log line at startup should no longer appear if the limit is sufficient.

If you are running under a systemd service, you can instead add `LimitNOFILE=65535` option
to the `[Service]` section of your service unit file.

## DNS caching

Dendrite has a built-in DNS cache which significantly reduces the load that Dendrite will
place on your DNS resolver. This may also speed up outbound federation.

Consider enabling the DNS cache by modifying the `global` section of your configuration file:

```yaml
  dns_cache:
    enabled: true
    cache_size: 4096
    cache_lifetime: 600s
```

## Time synchronisation

Matrix relies heavily on TLS which requires the system time to be correct. If the clock
drifts then you may find that federation no works reliably (or at all) and clients may
struggle to connect to your Dendrite server.

Ensure that the time is synchronised on your system by enabling NTP sync.
