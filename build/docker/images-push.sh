#!/bin/bash

TAG=${1:-latest}

echo "Pushing tag '${TAG}'"

docker push matrixdotorg/dendrite-monolith:${TAG}

docker push matrixdotorg/dendrite-appservice:${TAG}
docker push matrixdotorg/dendrite-clientapi:${TAG}
docker push matrixdotorg/dendrite-eduserver:${TAG}
docker push matrixdotorg/dendrite-federationapi:${TAG}
docker push matrixdotorg/dendrite-federationsender:${TAG}
docker push matrixdotorg/dendrite-keyserver:${TAG}
docker push matrixdotorg/dendrite-mediaapi:${TAG}
docker push matrixdotorg/dendrite-roomserver:${TAG}
docker push matrixdotorg/dendrite-syncapi:${TAG}
docker push matrixdotorg/dendrite-signingkeyserver:${TAG}
docker push matrixdotorg/dendrite-userapi:${TAG}
