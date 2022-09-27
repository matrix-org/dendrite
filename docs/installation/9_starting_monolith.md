---
title: Starting the monolith
parent: Installation
has_toc: true
nav_order: 9
permalink: /installation/start/monolith
---

# Starting the monolith

Once you have completed all of the preparation and installation steps,
you can start your Dendrite monolith deployment by starting the `dendrite-monolith-server`:

```bash
./dendrite-monolith-server -config /path/to/dendrite.yaml
```

By default, Dendrite will listen HTTP on port 8008. If you want to change the addresses
or ports that Dendrite listens on, you can use the `-http-bind-address` and
`-https-bind-address` command line arguments:

```bash
./dendrite-monolith-server -config /path/to/dendrite.yaml \
    -http-bind-address 1.2.3.4:12345 \
    -https-bind-address 1.2.3.4:54321
```

## Running under systemd

A common deployment pattern is to run the monolith under systemd. For this, you
will need to create a service unit file. An example service unit file is available
in the [GitHub repository](https://github.com/matrix-org/dendrite/blob/main/docs/systemd/monolith-example.service).

Once you have installed the service unit, you can notify systemd, enable and start
the service:

```bash
systemctl daemon-reload
systemctl enable dendrite
systemctl start dendrite
journalctl -fu dendrite
```
