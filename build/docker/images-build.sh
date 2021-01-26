#!/bin/bash

cd $(git rev-parse --show-toplevel)

TAG=${1:-latest}

echo "Building tag '${TAG}'"

docker build -t matrixdotorg/dendrite-monolith:${TAG}  -f build/docker/Dockerfile.monolith .
docker build -t matrixdotorg/dendrite-polylith:${TAG}  -f build/docker/Dockerfile.polylith .