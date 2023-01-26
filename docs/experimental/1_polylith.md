---
title: Installing as a polylith
parent: Experimental
nav_order: 1
permalink: /installation/experimental/polylith
---

# Installing as a polylith

## Using docker-compose

Dendrite provides an [example](https://github.com/matrix-org/dendrite/blob/main/build/docker/docker-compose.polylith.yml)
Docker compose file, which needs some preparation to start successfully.
Please note that this compose file has Postgres and NATS JetStream as a dependency, and you need to configure
a [reverse proxy](../planning#reverse-proxy).

### Preparations

Note that we're going to use the `dendrite-monolith` image in the next steps, as the `dendrite-polylith` image does
not provide the needed binaries to generate keys and configs.

#### Generate a private key

First we'll generate private key, which is used to sign events, the following will create one in `./config`:

```bash
mkdir -p ./config
docker run --rm --entrypoint="/usr/bin/generate-keys" \
  -v $(pwd)/config:/mnt \
  matrixdotorg/dendrite-monolith:latest \
  -private-key /mnt/matrix_key.pem
```
(**NOTE**: This only needs to be executed **once**, as you otherwise overwrite the key)

#### Generate a config

Similar to the command above, we can generate a config to be used, which will use the correct paths
as specified in the example docker-compose file. Change `server` to your domain and `db` according to your changes
to the docker-compose file (`services.postgres.environment` values):

```bash
mkdir -p ./config
docker run --rm --entrypoint="/bin/sh" \
  -v $(pwd)/config:/mnt \
  matrixdotorg/dendrite-monolith:latest \
  -c "/usr/bin/generate-config \
    -polylith \
    -db postgres://dendrite:itsasecret@postgres/dendrite?sslmode=disable \
    -server YourDomainHere > /mnt/dendrite.yaml"
```

We now need to modify the generated config, since `-polylith` generates one to be used on the same machine:

Set the Jetstream configuration to:
```yaml
global:
  jetstream:
    storage_path: /var/dendrite/
    addresses:
      - nats://jetstream:4222
    topic_prefix: Dendrite
    in_memory: false
    disable_tls_validation: false # Required when using the example compose file
```

For each component defined, remove the `internal_api.listen` hostname (`localhost`) and change the `internal_api.connect` hostname
to the corresponding hostname from the `docker-compose.polylith.yml`, e.g. the result for `user_api`:

```yaml
user_api:
  internal_api:
    listen: http://:7781
    connect: http://user_api:7781
```


#### Starting Dendrite

Once you're done changing the config, you can now start up Dendrite with

```bash
docker-compose -f docker-compose.polylith.yml up 
```

## Manual installation

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

### Reverse proxy

A reverse proxy such as [Caddy](https://caddyserver.com), [NGINX](https://www.nginx.com) or
[HAProxy](http://www.haproxy.org) is required for polylith deployments.
Configuring those not covered in this documentation, although sample configurations
for [Caddy](https://github.com/matrix-org/dendrite/blob/main/docs/caddy) and
[NGINX](https://github.com/matrix-org/dendrite/blob/main/docs/nginx) are provided.


### NATS Server

Polylith deployments currently need a standalone NATS Server installation with JetStream
enabled.

To do so, follow the [NATS Server installation instructions](https://docs.nats.io/running-a-nats-service/introduction/installation) and then [start your NATS deployment](https://docs.nats.io/running-a-nats-service/introduction/running). JetStream must be enabled, either by passing the `-js` flag to `nats-server`,
or by specifying the `store_dir` option in the the `jetstream` configuration.

### Starting the polylith

Once you have completed all preparation and installation steps,
you can start your Dendrite polylith deployment by starting the various components
using the `dendrite-polylith-multi` personalities.

### Starting the components

Each component must be started individually:

#### Client API

```bash
./dendrite-polylith-multi -config /path/to/dendrite.yaml clientapi
```

#### Sync API

```bash
./dendrite-polylith-multi -config /path/to/dendrite.yaml syncapi
```

#### Media API

```bash
./dendrite-polylith-multi -config /path/to/dendrite.yaml mediaapi
```

#### Federation API

```bash
./dendrite-polylith-multi -config /path/to/dendrite.yaml federationapi
```

#### Roomserver

```bash
./dendrite-polylith-multi -config /path/to/dendrite.yaml roomserver
```

#### Appservice API

```bash
./dendrite-polylith-multi -config /path/to/dendrite.yaml appservice
```

#### User API

```bash
./dendrite-polylith-multi -config /path/to/dendrite.yaml userapi
```

#### Key server

```bash
./dendrite-polylith-multi -config /path/to/dendrite.yaml keyserver
```
