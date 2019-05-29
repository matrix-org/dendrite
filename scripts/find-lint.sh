#! /bin/bash

# Runs the linters against dendrite

# The linters can take a lot of resources and are slow, so they can be
# configured using two environment variables:
#
# - `DENDRITE_LINT_CONCURRENCY` - number of concurrent linters to run,
#   golangci-lint defaults this to NumCPU


set -eux

cd `dirname $0`/..

args=""
if [ ${1:-""} = "fast" ]
then args="--fast"
fi

if [ -n "${DENDRITE_LINT_CONCURRENCY:-}" ]
then args="$args --concurrency=$DENDRITE_LINT_CONCURRENCY"
fi

echo "GOGC: $GOGC"

echo "Installing golangci-lint..."
go get github.com/golangci/golangci-lint/cmd/golangci-lint

echo "Looking for lint..."
golangci-lint run $args

echo "Checking spelling..."
misspell -error src *.md
