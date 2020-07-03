#!/bin/bash

docker pull matrixdotorg/dendrite:monolith

docker pull matrixdotorg/dendrite:appservice
docker pull matrixdotorg/dendrite:clientapi
docker pull matrixdotorg/dendrite:clientproxy
docker pull matrixdotorg/dendrite:eduserver
docker pull matrixdotorg/dendrite:federationapi
docker pull matrixdotorg/dendrite:federationsender
docker pull matrixdotorg/dendrite:federationproxy
docker pull matrixdotorg/dendrite:keyserver
docker pull matrixdotorg/dendrite:mediaapi
docker pull matrixdotorg/dendrite:currentstateserver
docker pull matrixdotorg/dendrite:roomserver
docker pull matrixdotorg/dendrite:syncapi
docker pull matrixdotorg/dendrite:userapi
