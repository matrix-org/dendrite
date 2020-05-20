#!/bin/bash

cd $(git rev-parse --show-toplevel)

COMPONENTS=$(ls docker/hub/Dockerfile.* | cut -d "." -f2)
for NAME in $COMPONENTS; do
	docker push matrixdotorg/dendrite:$NAME
done