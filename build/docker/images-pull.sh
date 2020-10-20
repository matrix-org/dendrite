#!/bin/bash

TAG=${1:-latest}

echo "Pulling tag '${TAG}'"

docker pull matrixdotorg/dendrite-monolith:${TAG}

docker pull matrixdotorg/dendrite-appservice:${TAG}
docker pull matrixdotorg/dendrite-clientapi:${TAG}
docker pull matrixdotorg/dendrite-eduserver:${TAG}
docker pull matrixdotorg/dendrite-federationapi:${TAG}
docker pull matrixdotorg/dendrite-federationsender:${TAG}
docker pull matrixdotorg/dendrite-keyserver:${TAG}
docker pull matrixdotorg/dendrite-mediaapi:${TAG}
docker pull matrixdotorg/dendrite-roomserver:${TAG}
docker pull matrixdotorg/dendrite-syncapi:${TAG}
docker pull matrixdotorg/dendrite-signingkeyserver:${TAG}
docker pull matrixdotorg/dendrite-userapi:${TAG}
