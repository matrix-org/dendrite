#! /bin/bash

# Runs the linters against dendrite

# The linters can take a lot of resources and are slow, so they can be
# configured using the following environment variables:
#
# - `DENDRITE_LINT_CONCURRENCY` - number of concurrent linters to run,
#   golangci-lint defaults this to NumCPU
# - `GOGC` - how often to perform garbage collection during golangci-lint runs.
#   Essentially a ratio of memory/speed. See https://github.com/golangci/golangci-lint#memory-usage-of-golangci-lint
#   for more info.


set -eux

cd `dirname $0`/..

args=""
if [ ${1:-""} = "fast" ]
then args="--fast"
fi

echo "Installing golangci-lint..."
go get github.com/golangci/golangci-lint/cmd/golangci-lint

echo "Looking for lint..."
golangci-lint run $args
