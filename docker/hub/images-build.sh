#!/bin/bash

cd $(git rev-parse --show-toplevel)

# go run ./cmd/generate-keys --private-key=docker/hub/config/matrix_key.pem -tls-cert=docker/hub/config/server.crt -tls-key=docker/hub/config/server.key

docker build -f docker/hub/Dockerfile -t matrixdotorg/dendrite:latest .

COMPONENTS=$(ls docker/hub/Dockerfile.* | cut -d "." -f2)
for NAME in $COMPONENTS; do
	docker build -f docker/hub/Dockerfile.$NAME -t matrixdotorg/dendrite:$NAME .
done