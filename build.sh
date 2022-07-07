#!/bin/sh -eu

# Put installed packages into ./bin
export GOBIN=$PWD/`dirname $0`/bin

if [ -d ".git" ] 
then
    export BUILD=`git rev-parse --short HEAD || ""`
    export BRANCH=`(git symbolic-ref --short HEAD | tr -d \/ ) || ""`
    if [ "$BRANCH" = main ]
    then
        export BRANCH=""
    fi

    export FLAGS="-X github.com/matrix-org/dendrite/internal.branch=$BRANCH -X github.com/matrix-org/dendrite/internal.build=$BUILD"
else
    export FLAGS=""
fi

mkdir -p bin

CGO_ENABLED=1 go build -trimpath -ldflags "$FLAGS" -v -o "bin/" ./cmd/...

# CGO_ENABLED=0 GOOS=js GOARCH=wasm go build -trimpath -ldflags "$FLAGS" -o bin/main.wasm ./cmd/dendritejs-pinecone
