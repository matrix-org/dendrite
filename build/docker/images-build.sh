#!/bin/bash

cd $(git rev-parse --show-toplevel)

docker build -f build/docker/Dockerfile -t matrixdotorg/dendrite:latest .

docker build -t matrixdotorg/dendrite-monolith:latest          --build-arg component=dendrite-monolith-server          -f build/docker/Dockerfile.component .

docker build -t matrixdotorg/dendrite-appservice:latest        --build-arg component=dendrite-appservice-server        -f build/docker/Dockerfile.component .
docker build -t matrixdotorg/dendrite-clientapi:latest         --build-arg component=dendrite-client-api-server        -f build/docker/Dockerfile.component .
docker build -t matrixdotorg/dendrite-eduserver:latest         --build-arg component=dendrite-edu-server               -f build/docker/Dockerfile.component .
docker build -t matrixdotorg/dendrite-federationapi:latest     --build-arg component=dendrite-federation-api-server    -f build/docker/Dockerfile.component .
docker build -t matrixdotorg/dendrite-federationsender:latest  --build-arg component=dendrite-federation-sender-server -f build/docker/Dockerfile.component .
docker build -t matrixdotorg/dendrite-keyserver:latest         --build-arg component=dendrite-key-server               -f build/docker/Dockerfile.component .
docker build -t matrixdotorg/dendrite-mediaapi:latest          --build-arg component=dendrite-media-api-server         -f build/docker/Dockerfile.component .
docker build -t matrixdotorg/dendrite-roomserver:latest        --build-arg component=dendrite-room-server              -f build/docker/Dockerfile.component .
docker build -t matrixdotorg/dendrite-syncapi:latest           --build-arg component=dendrite-sync-api-server          -f build/docker/Dockerfile.component .
docker build -t matrixdotorg/dendrite-signingkeyserver:latest  --build-arg component=dendrite-signing-key-server       -f build/docker/Dockerfile.component .
docker build -t matrixdotorg/dendrite-userapi:latest           --build-arg component=dendrite-user-api-server          -f build/docker/Dockerfile.component .
