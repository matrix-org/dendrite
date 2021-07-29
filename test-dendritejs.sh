#!/bin/sh -eu

GOOS=js GOARCH=wasm go test -v -exec "$(pwd)/test/wasm/index.js" ./cmd/dendritejs-pinecone
