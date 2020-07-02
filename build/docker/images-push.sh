#!/bin/bash

docker push matrixdotorg/dendrite:monolith

docker push matrixdotorg/dendrite:appservice
docker push matrixdotorg/dendrite:clientapi
docker push matrixdotorg/dendrite:clientproxy
docker push matrixdotorg/dendrite:eduserver
docker push matrixdotorg/dendrite:federationapi
docker push matrixdotorg/dendrite:federationsender
docker push matrixdotorg/dendrite:federationproxy
docker push matrixdotorg/dendrite:keyserver
docker push matrixdotorg/dendrite:mediaapi
docker push matrixdotorg/dendrite:currentstateserver
docker push matrixdotorg/dendrite:roomserver
docker push matrixdotorg/dendrite:syncapi
docker push matrixdotorg/dendrite:serverkeyapi
docker push matrixdotorg/dendrite:userapi
