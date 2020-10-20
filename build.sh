#!/bin/bash -eu

# Put installed packages into ./bin
export GOBIN=$PWD/`dirname $0`/bin

if [ -d ".git" ] 
then
    export BUILD=`git rev-parse --short HEAD || ""`
    export BRANCH=`(git symbolic-ref --short HEAD | tr -d \/ ) || ""`
    if [[ $BRANCH == "master" ]]
    then
        export BRANCH=""
    fi

    export FLAGS="-X github.com/matrix-org/dendrite/internal.branch=$BRANCH -X github.com/matrix-org/dendrite/internal.build=$BUILD"
else
    export FLAGS=""
fi

go install -trimpath -ldflags "$FLAGS" -v $PWD/`dirname $0`/cmd/...

GOOS=js GOARCH=wasm go build -trimpath -ldflags "$FLAGS" -o main.wasm ./cmd/dendritejs
