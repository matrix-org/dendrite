#!/bin/bash -eu

# Put installed packages into ./bin
export GOBIN=$PWD/`dirname $0`/bin

export BRANCH=`(git symbolic-ref --short HEAD | cut -d'/' -f 3 )|| ""`
export BUILD=`git rev-parse --short HEAD || ""`

export FLAGS="-X github.com/matrix-org/dendrite/internal.branch=$BRANCH -X github.com/matrix-org/dendrite/internal.build=$BUILD"

go install -trimpath -ldflags "$FLAGS" -v $PWD/`dirname $0`/cmd/...

GOOS=js GOARCH=wasm go build -trimpath -ldflags "$FLAGS" -o main.wasm ./cmd/dendritejs
