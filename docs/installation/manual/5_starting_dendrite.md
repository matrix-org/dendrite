---
title: Starting Dendrite
parent: Manual
grand_parent: Installation
nav_order: 5
permalink: /installation/manual/start
---

# Starting Dendrite

Once you have completed all preparation and installation steps,
you can start your Dendrite deployment by executing the `dendrite` binary:

```bash
./dendrite -config /path/to/dendrite.yaml
```

By default, Dendrite will listen HTTP on port 8008. If you want to change the addresses
or ports that Dendrite listens on, you can use the `-http-bind-address` and
`-https-bind-address` command line arguments:

```bash
./dendrite -config /path/to/dendrite.yaml \
    -http-bind-address 1.2.3.4:12345 \
    -https-bind-address 1.2.3.4:54321
```
