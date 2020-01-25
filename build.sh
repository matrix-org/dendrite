#!/bin/sh

GOOS=js GOARCH=wasm GOBIN=$PWD/`dirname $0`/bin go build -v $PWD/`dirname $0`/cmd/dendrite-monolith-server
