#!/bin/bash -eu

# Put installed packages into ./bin
export GOBIN=$PWD/`dirname $0`/bin

go install -v $PWD/`dirname $0`/cmd/...