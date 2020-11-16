#!/bin/sh -eu

export GIT_COMMIT=$(git rev-list -1 HEAD) && \
GOOS=js GOARCH=wasm go build -ldflags "-X main.GitCommit=$GIT_COMMIT" -o main.wasm ./cmd/dendritejs
