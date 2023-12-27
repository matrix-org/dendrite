---
title: Optimise your installation
parent: Installation
has_toc: true
nav_order: 10
permalink: /installation/start/optimisation
---

# Optimise your installation

Now that you have Dendrite running, the following tweaks will improve the reliability
and performance of your installation.

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
