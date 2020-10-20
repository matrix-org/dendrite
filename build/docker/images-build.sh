#!/bin/bash

cd $(git rev-parse --show-toplevel)

TAG=${1:-latest}

echo "Building tag '${TAG}'"

docker build -f build/docker/Dockerfile -t matrixdotorg/dendrite:${TAG} .

docker build -t matrixdotorg/dendrite-monolith:${TAG}          --build-arg component=dendrite-monolith-server          -f build/docker/Dockerfile.component .

docker build -t matrixdotorg/dendrite-appservice:${TAG}        --build-arg component=dendrite-appservice-server        -f build/docker/Dockerfile.component .
docker build -t matrixdotorg/dendrite-clientapi:${TAG}         --build-arg component=dendrite-client-api-server        -f build/docker/Dockerfile.component .
docker build -t matrixdotorg/dendrite-eduserver:${TAG}         --build-arg component=dendrite-edu-server               -f build/docker/Dockerfile.component .
docker build -t matrixdotorg/dendrite-federationapi:${TAG}     --build-arg component=dendrite-federation-api-server    -f build/docker/Dockerfile.component .
docker build -t matrixdotorg/dendrite-federationsender:${TAG}  --build-arg component=dendrite-federation-sender-server -f build/docker/Dockerfile.component .
docker build -t matrixdotorg/dendrite-keyserver:${TAG}         --build-arg component=dendrite-key-server               -f build/docker/Dockerfile.component .
docker build -t matrixdotorg/dendrite-mediaapi:${TAG}          --build-arg component=dendrite-media-api-server         -f build/docker/Dockerfile.component .
docker build -t matrixdotorg/dendrite-roomserver:${TAG}        --build-arg component=dendrite-room-server              -f build/docker/Dockerfile.component .
docker build -t matrixdotorg/dendrite-syncapi:${TAG}           --build-arg component=dendrite-sync-api-server          -f build/docker/Dockerfile.component .
docker build -t matrixdotorg/dendrite-signingkeyserver:${TAG}  --build-arg component=dendrite-signing-key-server       -f build/docker/Dockerfile.component .
docker build -t matrixdotorg/dendrite-userapi:${TAG}           --build-arg component=dendrite-user-api-server          -f build/docker/Dockerfile.component .
