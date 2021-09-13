#!/bin/sh -eu

# Put installed packages into ./bin
export GOBIN=$PWD/`dirname $0`/bin
export FLAGS=""

mkdir -p bin

CGO_ENABLED=1 go build -trimpath -ldflags "$FLAGS" -v -o "bin/" ./cmd/...

CGO_ENABLED=0 GOOS=js GOARCH=wasm go build -trimpath -ldflags "$FLAGS" -o bin/main.wasm ./cmd/dendritejs-pinecone
