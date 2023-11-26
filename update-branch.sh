#! /usr/bin/env sh

#
# This is because matrix-org doesn't allow anonymous contributors.
#
echo "Pulling in changes from matrix-org main"
git pull origin main
echo "Pulling in changes from eyedeekay i2p-demo"
git pull idk i2p-demo
echo "Generating binary"
go build -o bin/dendrite-demo-i2p ./cmd/dendrite-demo-i2p