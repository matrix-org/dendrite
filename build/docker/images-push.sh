#!/usr/bin/env bash

TAG=${1:-latest}

echo "Pushing tag '${TAG}'"

docker push matrixdotorg/dendrite-monolith:${TAG}
docker push matrixdotorg/dendrite-polylith:${TAG}