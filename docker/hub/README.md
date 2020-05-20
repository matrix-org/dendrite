# Docker Hub 

## Building the images

To start with, from the root of the Dendrite repository, build the Docker images:

```
sh docker/hub/build.sh
```

## Starting a monolith

Create some config:

```
go run ./cmd/generate-keys \
  --private-key=docker/hub/config/matrix_key.pem \
  --tls-cert=docker/hub/config/server.crt \
  --tls-key=docker/hub/config/server.key
cp docker/hub/config/dendrite-docker.yaml docker/hub/config/dendrite.yml
```

Start the dependencies:

```
docker-compose -f docker/hub/docker-compose.deps.yml
```

... and start a monolith deployment:

```
docker-compose -f docker/hub/docker-compose.monolith.yml
```

... or a polylith deployment:

```
docker-compose -f docker/hub/docker-compose.polylith.yml
```