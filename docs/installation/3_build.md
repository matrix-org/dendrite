---
title: Building Dendrite
parent: Installation
has_toc: true
nav_order: 3
permalink: /installation/build
---

# Build all Dendrite commands

Dendrite has numerous utility commands in addition to the actual server binaries.
Build them all from the root of the source repo with `build.sh` (Linux/Mac):

```sh
./build.sh
```

or `build.cmd` (Windows):

```powershell
build.cmd
```

The resulting binaries will be placed in the `bin` subfolder.

# Installing as a monolith

You can install the Dendrite monolith binary into `$GOPATH/bin` by using `go install`:

```sh
go install ./cmd/dendrite-monolith-server
```

Alternatively, you can specify a custom path for the binary to be written to using `go build`:

```sh
go build -o /usr/local/bin/ ./cmd/dendrite-monolith-server
```
