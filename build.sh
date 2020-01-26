#!/bin/sh

GOOS=js GOARCH=wasm GOBIN=$PWD/`dirname $0`/bin go build -v -o bin/dendrite.wasm $PWD/`dirname $0`/cmd/dendrite-monolith-server
