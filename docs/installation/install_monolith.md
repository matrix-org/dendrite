---
title: Installing as a monolith
parent: Installation
has_toc: true
nav_order: 5
---

# Installing as a monolith

On UNIX systems, the `build.sh` script will build all variants of Dendrite.

```bash
./build.sh
```

The `bin` directory will contain the built binaries.

For monolith deployments, the relevant binary is the `dendrite-monolith-server`
binary.

Copy the `./bin/dendrite-monolith-server` into a relevant system path, for example:

```bash
cp ./bin/dendrite-monolith-server /usr/local/bin/
```
