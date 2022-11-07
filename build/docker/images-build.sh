#!/usr/bin/env bash

cd $(git rev-parse --show-toplevel)

TAG=${1:-latest}

echo "Building tag '${TAG}'"

docker build . --target monolith -t matrixdotorg/dendrite-monolith:${TAG}
docker build . --target polylith -t matrixdotorg/dendrite-monolith:${TAG}
docker build . --target demo-pinecone -t matrixdotorg/dendrite-demo-pinecone:${TAG}
docker build . --target demo-yggdrasil -t matrixdotorg/dendrite-demo-yggdrasil:${TAG}