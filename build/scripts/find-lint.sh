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

# Make a backup of go.{mod,sum} first
# TODO: Once go 1.13 is out, use go get's -mod=readonly option
# https://github.com/golang/go/issues/30667
cp go.mod go.mod.bak && cp go.sum go.sum.bak
go get github.com/golangci/golangci-lint/cmd/golangci-lint@v1.19.1

# Run linting
echo "Looking for lint..."

# Capture exit code to ensure go.{mod,sum} is restored before exiting
exit_code=0

golangci-lint run $args || exit_code=1

# Restore go.{mod,sum}
mv go.mod.bak go.mod && mv go.sum.bak go.sum

exit $exit_code
