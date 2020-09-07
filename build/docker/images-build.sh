#!/bin/bash

cd $(git rev-parse --show-toplevel)

docker build -f build/docker/Dockerfile -t matrixdotorg/dendrite:latest .

docker build -t matrixdotorg/dendrite:monolith          --build-arg component=dendrite-monolith-server          -f build/docker/Dockerfile.component .

docker build -t matrixdotorg/dendrite:appservice        --build-arg component=dendrite-appservice-server        -f build/docker/Dockerfile.component .
docker build -t matrixdotorg/dendrite:clientapi         --build-arg component=dendrite-client-api-server        -f build/docker/Dockerfile.component .
docker build -t matrixdotorg/dendrite:clientproxy       --build-arg component=client-api-proxy                  -f build/docker/Dockerfile.component .
docker build -t matrixdotorg/dendrite:eduserver         --build-arg component=dendrite-edu-server               -f build/docker/Dockerfile.component .
docker build -t matrixdotorg/dendrite:federationapi     --build-arg component=dendrite-federation-api-server    -f build/docker/Dockerfile.component .
docker build -t matrixdotorg/dendrite:federationsender  --build-arg component=dendrite-federation-sender-server -f build/docker/Dockerfile.component .
docker build -t matrixdotorg/dendrite:federationproxy   --build-arg component=federation-api-proxy              -f build/docker/Dockerfile.component .
docker build -t matrixdotorg/dendrite:keyserver         --build-arg component=dendrite-key-server               -f build/docker/Dockerfile.component .
docker build -t matrixdotorg/dendrite:mediaapi          --build-arg component=dendrite-media-api-server         -f build/docker/Dockerfile.component .
docker build -t matrixdotorg/dendrite:roomserver        --build-arg component=dendrite-room-server              -f build/docker/Dockerfile.component .
docker build -t matrixdotorg/dendrite:syncapi           --build-arg component=dendrite-sync-api-server          -f build/docker/Dockerfile.component .
docker build -t matrixdotorg/dendrite:serverkeyapi      --build-arg component=dendrite-server-key-api-server    -f build/docker/Dockerfile.component .
docker build -t matrixdotorg/dendrite:userapi           --build-arg component=dendrite-user-api-server          -f build/docker/Dockerfile.component .
