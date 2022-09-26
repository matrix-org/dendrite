#!/usr/bin/env bash

TAG=${1:-latest}

echo "Pulling tag '${TAG}'"

docker pull matrixdotorg/dendrite-monolith:${TAG}
docker pull matrixdotorg/dendrite-polylith:${TAG}