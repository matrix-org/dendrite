#! /bin/bash

# Runs the linters against dendrite

# The linters can take a lot of resources and are slow, so they can be
# configured using two environment variables:
#
# - `DENDRITE_LINT_CONCURRENCY` - number of concurrent linters to run,
#   gometalinter defaults this to 8
# - `DENDRITE_LINT_DISABLE_GC` - if set then the the go gc will be disabled
#   when running the linters, speeding them up but using much more memory.


set -eux

cd `dirname $0`/..

export GOPATH=$(pwd)/vendor

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
curl -sfL https://install.goreleaser.com/github.com/golangci/golangci-lint.sh | bash -s v1.6.1
go get github.com/client9/misspell/cmd/misspell
export GOPATH="$GOPATH:$(pwd)"

echo "Looking for lint..."
golangci-lint run --max-same-issues 0 --max-issues-per-linter 0

echo "Double checking spelling..."
$(pwd)/vendor/bin/misspell -error src *.md