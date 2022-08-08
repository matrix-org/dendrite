---
title: Installing as a polylith
parent: Installation
has_toc: true
nav_order: 6
permalink: /installation/install/polylith
---

# Installing as a polylith

You can install the Dendrite polylith binary into `$GOPATH/bin` by using `go install`:

```sh
go install ./cmd/dendrite-polylith-multi
```

Alternatively, you can specify a custom path for the binary to be written to using `go build`:

```sh
go build -o /usr/local/bin/ ./cmd/dendrite-polylith-multi
```

The `dendrite-polylith-multi` binary is a "multi-personality" binary which can run as
any of the components depending on the supplied command line parameters.

## Reverse proxy

Polylith deployments require a reverse proxy in order to ensure that requests are
sent to the correct endpoint. You must ensure that a suitable reverse proxy is installed
and configured.

Sample configurations are provided
for [Caddy](https://github.com/matrix-org/dendrite/blob/main/docs/caddy/polylith/Caddyfile)
and [NGINX](https://github.com/matrix-org/dendrite/blob/main/docs/nginx/polylith-sample.conf).