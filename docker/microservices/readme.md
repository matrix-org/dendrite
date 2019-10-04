# Docker Images

## Build

The main `Dockerfile` builds installs all go modules and builds all the go binaries in `golang:1.12.9`, and should be tagged `dendrite:latest`. This tag is ensures that it cannot be pushed, and a CI/CD server should not have it cached.

All the other containers are generated from the binaries built in the first container. It copies the binary from `dendrite:latest` to the root of `alpine:3.10`.

## Usage

## Environment Variables

```yaml
# bind-address arg to client-api-proxy
client-bind-address
# bind-address arg to federation-api-proxy
federation-bind-address

client-api-server-url
sync-api-server-url
media-api-server-url
public-rooms-api-server-url
federation-api-url
config
```
