#!/bin/bash -eu

# Put installed packages into ./bin
export GOBIN=$PWD/`dirname $0`/bin

# We can't build all packages ala ./cmd/... because ./cmd/dendritejs only works for WASM
find ./cmd -mindepth 1 -maxdepth 1 -type d | grep -v dendritejs | xargs go install -v

# worth compiling it to make sure it's happy though, and who knows maybe they do want to build for wasm ;)
GOOS=js GOARCH=wasm go build -o dendritejs.wasm $PWD/`dirname $0`/cmd/dendritejs