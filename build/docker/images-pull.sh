#!/bin/bash

docker pull matrixdotorg/dendrite-monolith:latest

docker pull matrixdotorg/dendrite-appservice:latest
docker pull matrixdotorg/dendrite-clientapi:latest
docker pull matrixdotorg/dendrite-eduserver:latest
docker pull matrixdotorg/dendrite-federationapi:latest
docker pull matrixdotorg/dendrite-federationsender:latest
docker pull matrixdotorg/dendrite-keyserver:latest
docker pull matrixdotorg/dendrite-mediaapi:latest
docker pull matrixdotorg/dendrite-roomserver:latest
docker pull matrixdotorg/dendrite-syncapi:latest
docker pull matrixdotorg/dendrite-signingkeyserver:latest
docker pull matrixdotorg/dendrite-userapi:latest
