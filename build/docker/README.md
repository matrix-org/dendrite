# Docker images

These are Docker images for Dendrite!

## Dockerfiles

The `Dockerfile` builds the base image which contains all of the Dendrite
components. The `Dockerfile.component` file takes the given component, as
specified with `--buildarg component=` from the base image and produce
smaller component-specific images, which are substantially smaller and do
not contain the Go toolchain etc.

## Compose files

There are three sample `docker-compose` files:

- `docker-compose.deps.yml` which runs the Postgres and Kafka prerequisites
- `docker-compose.monolith.yml` which runs a monolith Dendrite deployment
- `docker-compose.polylith.yml` which runs a polylith Dendrite deployment

## Configuration

The `docker-compose` files refer to the `/etc/dendrite` volume as where the
runtime config should come from. The mounted folder must contain:

- `dendrite.yaml` configuration file (based on the sample `dendrite-config.yaml`
   in the `docker/config` folder in the [Dendrite repository](https://github.com/matrix-org/dendrite)
- `matrix_key.pem` server key, as generated using `cmd/generate-keys`
- `server.crt` certificate file
- `server.key` private key file for the above certificate

To generate keys:

```
go run github.com/matrix-org/dendrite/cmd/generate-keys \
  --private-key=matrix_key.pem \
  --tls-cert=server.crt \
  --tls-key=server.key
```

## Starting Dendrite

Once in place, start the dependencies:

```
docker-compose -f docker-compose.deps.yml up
```

Wait a few seconds for Kafka and Postgres to finish starting up, and then start a monolith:

```
docker-compose -f docker-compose.monolith.yml up
```

... or start the polylith components:

```
docker-compose -f docker-compose.polylith.yml up
```

## Building the images

The `docker/images-build.sh` script will build the base image, followed by
all of the component images.

The `docker/images-push.sh` script will push them to Docker Hub (subject
to permissions).

If you wish to build and push your own images, rename `matrixdotorg/dendrite` to
the name of another Docker Hub repository in `images-build.sh` and `images-push.sh`.
