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

If you intend to run a separate NATS server, start it now. Otherwise make sure that [`dendrite.yaml`] is configured to use the in-process NATS server.

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

## Proxying dendrite

Container deployments do often use a proxying webserver e.g. to implement various vhosts such as `example.com`, `www.example.com`, `images.example.com` and `matrix.example.com` on the same IP address. It is possible to use dendrite in such a setup.

If you intend to use a webserver such as [Apache](https://apache.org) as a frontend proxy, make sure that it does not canonicalize URLs. Otherwise chats might fail because signatures on federated requests will not match (see https://github.com/matrix-org/synapse/issues/3294 ). An example vhost configuration might look as follows:

```
<VirtualHost matrix.example.com:443>
        ServerName matrix.example.com

        SSLEngine on
        SSLCertificateFile      /etc/ssl/matrix.example.com/cert.pem
        SSLCertificateKeyFile   /etc/ssl/matrix.example.com/privkey.pem
        SSLCertificateChainFile /etc/ssl/matrix.example.com/chain.pem

        # Container uses a unique non-signed certificate. 
        SSLProxyEngine On
        SSLProxyVerify None
        SSLProxyCheckPeerCN Off
        SSLProxyCheckPeerName Off

        SetEnvIf Host "^(.*)$" THE_HOST=$1
        RequestHeader setifempty X-Forwarded-Proto https
        RequestHeader setifempty X-Forwarded-Host %{THE_HOST}e
        ProxyAddHeaders Off

        ProxyPass           / https://127.0.0.1:8448/ nocanon
        ProxyPassReverse    / https://127.0.0.1:8448/
</VirtualHost>
```

The certificates specified need to at least be valid for `matrix.example.com` (or whatever your homeserver hostname/vhost should be).

