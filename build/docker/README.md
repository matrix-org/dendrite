# Docker images

These are Docker images for Dendrite!

They can be found on Docker Hub:

- [matrixdotorg/dendrite-monolith](https://hub.docker.com/r/matrixdotorg/dendrite-monolith) for monolith deployments
- [matrixdotorg/dendrite-polylith](https://hub.docker.com/r/matrixdotorg/dendrite-polylith) for polylith deployments

## Dockerfiles

The `Dockerfile` builds the base image which contains all of the Dendrite
components. The `Dockerfile.component` file takes the given component, as
specified with `--buildarg component=` from the base image and produce
smaller component-specific images, which are substantially smaller and do
not contain the Go toolchain etc.

## Compose files

There are three sample `docker-compose` files:

- `docker-compose.monolith.yml` which runs a monolith Dendrite deployment
- `docker-compose.polylith.yml` which runs a polylith Dendrite deployment

## Configuration

The `docker-compose` files refer to the `/etc/dendrite` volume as where the
runtime config should come from. The mounted folder must contain:

- `dendrite.yaml` configuration file (from the [Docker config folder](https://github.com/matrix-org/dendrite/tree/master/build/docker/config)
   sample in the `build/docker/config` folder of this repository.)
- `matrix_key.pem` server key, as generated using `cmd/generate-keys`
- `server.crt` certificate file
- `server.key` private key file for the above certificate

To generate keys:

```
docker run --rm --entrypoint="" \
  -v $(pwd):/mnt \
  matrixdotorg/dendrite-monolith:latest \
  /usr/bin/generate-keys \
  -private-key /mnt/matrix_key.pem \
  -tls-cert /mnt/server.crt \
  -tls-key /mnt/server.key
```

The key files will now exist in your current working directory, and can be mounted into place.

## Starting Dendrite as a monolith deployment

Create your config based on the [`dendrite.yaml`](https://github.com/matrix-org/dendrite/tree/master/build/docker/config) configuration file in the `build/docker/config` folder of this repository.

Then start the deployment:

```
docker-compose -f docker-compose.monolith.yml up
```

## Starting Dendrite as a polylith deployment

Create your config based on the [`dendrite-config.yaml`](https://github.com/matrix-org/dendrite/tree/master/build/docker/config) configuration file in the `build/docker/config` folder of this repository.

Then start the deployment:

```
docker-compose -f docker-compose.polylith.yml up
```

## Building the images

The `build/docker/images-build.sh` script will build the base image, followed by
all of the component images.

The `build/docker/images-push.sh` script will push them to Docker Hub (subject
to permissions).

If you wish to build and push your own images, rename `matrixdotorg/dendrite` to
the name of another Docker Hub repository in `images-build.sh` and `images-push.sh`.
