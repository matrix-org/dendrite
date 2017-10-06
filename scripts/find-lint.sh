#! /bin/bash

set -eu

export GOPATH="$(pwd):$(pwd)/vendor"
export PATH="$PATH:$(pwd)/vendor/bin:$(pwd)/bin"

args=""
if [ ${1:-""} = "fast" ]
then args="--config=linter-fast.json"
else args="--config=linter.json"
fi

if [ -n "${DENDRITE_LINT_CONCURRENCY:-}" ]
then args="$args --concurrency=$DENDRITE_LINT_CONCURRENCY"
fi

if [ -z "${DENDRITE_LINT_DISABLE_GC:-}" ]
then args="$args --enable-gc"
fi

echo "Installing lint search engine..."
go install github.com/alecthomas/gometalinter/
gometalinter --config=linter.json ./... --install

echo "Looking for lint..."
gometalinter ./... $args

echo "Double checking spelling..."
misspell -error src *.md

echo "Done!"
