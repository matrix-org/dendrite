---
title: Starting the polylith
parent: Installation
has_toc: true
nav_order: 10
permalink: /installation/start/polylith
---

# Starting the polylith

Once you have completed all of the preparation and installation steps,
you can start your Dendrite polylith deployment by starting the various components
using the `dendrite-polylith-multi` personalities.

## Start the reverse proxy

Ensure that your reverse proxy is started and is proxying the correct
endpoints to the correct components. Software such as [NGINX](https://www.nginx.com) or
[HAProxy](http://www.haproxy.org) can be used for this purpose. A [sample configuration
for NGINX](https://github.com/matrix-org/dendrite/blob/main/docs/nginx/polylith-sample.conf)
is provided.

## Starting the components

Each component must be started individually:

### Client API

```bash
./dendrite-polylith-multi -config /path/to/dendrite.yaml clientapi
```

### Sync API

```bash
./dendrite-polylith-multi -config /path/to/dendrite.yaml syncapi
```

### Media API

```bash
./dendrite-polylith-multi -config /path/to/dendrite.yaml mediaapi
```

### Federation API

```bash
./dendrite-polylith-multi -config /path/to/dendrite.yaml federationapi
```

### Roomserver

```bash
./dendrite-polylith-multi -config /path/to/dendrite.yaml roomserver
```

### Appservice API

```bash
./dendrite-polylith-multi -config /path/to/dendrite.yaml appservice
```

### User API

```bash
./dendrite-polylith-multi -config /path/to/dendrite.yaml userapi
```

### Key server

```bash
./dendrite-polylith-multi -config /path/to/dendrite.yaml keyserver
```
