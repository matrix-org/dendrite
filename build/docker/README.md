# Docker images

These are Docker images for Dendrite!

They can be found on Docker Hub:

- [matrixdotorg/dendrite-monolith](https://hub.docker.com/r/matrixdotorg/dendrite-monolith) for monolith deployments
- [matrixdotorg/dendrite-polylith](https://hub.docker.com/r/matrixdotorg/dendrite-polylith) for polylith deployments

## Building the image

The `Dockerfile` can build both monolith and polylith images for Dendrite.
To choose which is selected, set `--build-arg DENDRITE_BINARY=$binary`,
where `$binary` is `dendrite-monolith-server` or `dendrite-polylith-multi`
for monolith or polylith respectively.

Note that you **must** use BuildKit to build the images. You can enable
it for the build with `DOCKER_BUILDKIT=1 docker build [...]`

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

## Starting Dendrite as a monolith deployment

Create your config based on the `dendrite.yaml` configuration file in the `docker/config`
folder in the [Dendrite repository](https://github.com/matrix-org/dendrite). Additionally,
make the following changes to the configuration:

- Enable Naffka: `use_naffka: true`

Once in place, start the PostgreSQL dependency:

```
docker-compose -f docker-compose.deps.yml up postgres
```

Wait a few seconds for PostgreSQL to finish starting up, and then start a monolith:

```
docker-compose -f docker-compose.monolith.yml up
```

## Starting Dendrite as a polylith deployment

Create your config based on the `dendrite.yaml` configuration file in the `docker/config`
folder in the [Dendrite repository](https://github.com/matrix-org/dendrite).

Once in place, start all the dependencies:

```
docker-compose -f docker-compose.deps.yml up
```

Wait a few seconds for PostgreSQL and Kafka to finish starting up, and then start a polylith:

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
