#!/bin/bash

docker build matrixdotorg/dendrite:clientapi
docker build matrixdotorg/dendrite:clientproxy
docker build matrixdotorg/dendrite:eduserver
docker build matrixdotorg/dendrite:federationapi
docker build matrixdotorg/dendrite:federationsender
docker build matrixdotorg/dendrite:federationproxy
docker build matrixdotorg/dendrite:keyserver
docker build matrixdotorg/dendrite:mediaapi
docker build matrixdotorg/dendrite:publicroomsapi
docker build matrixdotorg/dendrite:roomserver
docker build matrixdotorg/dendrite:syncapi
