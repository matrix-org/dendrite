#!/bin/bash

docker push matrixdotorg/dendrite-monolith:latest

docker push matrixdotorg/dendrite-appservice:latest
docker push matrixdotorg/dendrite-clientapi:latest
docker push matrixdotorg/dendrite-eduserver:latest
docker push matrixdotorg/dendrite-federationapi:latest
docker push matrixdotorg/dendrite-federationsender:latest
docker push matrixdotorg/dendrite-keyserver:latest
docker push matrixdotorg/dendrite-mediaapi:latest
docker push matrixdotorg/dendrite-roomserver:latest
docker push matrixdotorg/dendrite-syncapi:latest
docker push matrixdotorg/dendrite-signingkeyserver:latest
docker push matrixdotorg/dendrite-userapi:latest
