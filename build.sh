#!/bin/bash -eu

# Put installed packages into ./bin
export GOBIN=$PWD/`dirname $0`/bin

go install -v $PWD/`dirname $0`/cmd/...

GOOS=js GOARCH=wasm go build -o main.wasm ./cmd/dendritejs
