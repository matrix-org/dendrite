---
title: Installation
parent: Docker
grand_parent: Installation
has_toc: true
nav_order: 1
permalink: /installation/docker/install
---

# Installing Dendrite using Docker Compose

Dendrite provides an [example](https://github.com/matrix-org/dendrite/blob/main/build/docker/docker-compose.yml) 
Docker compose file, which needs some preparation to start successfully.
Please note that this compose file only has Postgres as a dependency, and you need to configure
a [reverse proxy](../planning#reverse-proxy).

## Preparations

### Generate a private key

First we'll generate private key, which is used to sign events, the following will create one in `./config`:

```bash
mkdir -p ./config
docker run --rm --entrypoint="/usr/bin/generate-keys" \
  -v $(pwd)/config:/mnt \
  matrixdotorg/dendrite-monolith:latest \
  -private-key /mnt/matrix_key.pem
```
(**NOTE**: This only needs to be executed **once**, as you otherwise overwrite the key)

### Generate a config

Similar to the command above, we can generate a config to be used, which will use the correct paths
as specified in the example docker-compose file. Change `server` to your domain and `db` according to your changes
to the docker-compose file (`services.postgres.environment` values):

```bash
mkdir -p ./config
docker run --rm --entrypoint="/bin/sh" \
  -v $(pwd)/config:/mnt \
  matrixdotorg/dendrite-monolith:latest \
  -c "/usr/bin/generate-config \
    -dir /var/dendrite/ \
    -db postgres://dendrite:itsasecret@postgres/dendrite?sslmode=disable \
    -server YourDomainHere > /mnt/dendrite.yaml"
```

You can then change `config/dendrite.yaml` to your liking.

## Starting Dendrite

Once you're done changing the config, you can now start up Dendrite with

```bash
docker-compose -f docker-compose.yml up 
```
