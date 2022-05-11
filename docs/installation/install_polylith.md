---
title: Installing as a polylith
parent: Installation
has_toc: true
nav_order: 6
---

# Installing as a polylith

On UNIX systems, the `build.sh` script will build all variants of Dendrite.

```sh
./build.sh
```

The `bin` directory will contain the built binaries.

For polylith deployments, the relevant binary is the `dendrite-polylith-multi`
binary, which is a "multi-personality" binary which can run as any of the components
depending on the supplied command line parameters.

Copy the `./bin/dendrite-polylith-multi` into a relevant system path, for example:

```bash
cp ./bin/dendrite-polylith-multi /usr/local/bin/
```
