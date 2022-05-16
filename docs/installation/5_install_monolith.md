---
title: Installing as a monolith
parent: Installation
has_toc: true
nav_order: 5
permalink: /installation/install/monolith
---

# Installing as a monolith

You can install the Dendrite monolith binary into `$GOPATH/bin` by using `go install`:

```sh
go install ./cmd/dendrite-monolith-server
```

Alternatively, you can specify a custom path for the binary to be written to using `go build`:

```sh
go build -o /usr/local/bin/ ./cmd/dendrite-monolith-server
```
