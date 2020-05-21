#!/bin/bash

cd $(git rev-parse --show-toplevel)

docker build -f docker/hub/Dockerfile -t matrixdotorg/dendrite:latest .

docker build -t matrixdotorg/dendrite:clientapi 		--build-arg component=dendrite-client-api-server 		-f docker/hub/Dockerfile.component .
docker build -t matrixdotorg/dendrite:clientproxy 		--build-arg component=client-api-proxy	 				-f docker/hub/Dockerfile.component .
docker build -t matrixdotorg/dendrite:eduserver 		--build-arg component=dendrite-edu-server 				-f docker/hub/Dockerfile.component .
docker build -t matrixdotorg/dendrite:federationapi 	--build-arg component=dendrite-federation-api-server 	-f docker/hub/Dockerfile.component .
docker build -t matrixdotorg/dendrite:federationsender 	--build-arg component=dendrite-federation-sender-server -f docker/hub/Dockerfile.component .
docker build -t matrixdotorg/dendrite:federationproxy 	--build-arg component=federation-api-proxy	 			-f docker/hub/Dockerfile.component .
docker build -t matrixdotorg/dendrite:keyserver 		--build-arg component=dendrite-key-server		 		-f docker/hub/Dockerfile.component .
docker build -t matrixdotorg/dendrite:mediaapi 			--build-arg component=dendrite-media-api-server 		-f docker/hub/Dockerfile.component .
docker build -t matrixdotorg/dendrite:publicroomsapi 	--build-arg component=dendrite-public-rooms-api-server	-f docker/hub/Dockerfile.component .
docker build -t matrixdotorg/dendrite:roomserver 		--build-arg component=dendrite-room-server 				-f docker/hub/Dockerfile.component .
docker build -t matrixdotorg/dendrite:syncapi			--build-arg component=dendrite-sync-api-server 			-f docker/hub/Dockerfile.component .
