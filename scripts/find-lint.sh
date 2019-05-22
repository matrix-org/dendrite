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

# gometalinter doesn't seem to work without this.
# We should move from gometalinter asap as per https://github.com/matrix-org/dendrite/issues/697 so this is a temporary
# fix.
export GO111MODULE=off

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
go get github.com/alecthomas/gometalinter/

gometalinter --config=linter.json ./... --install

echo "Looking for lint..."
gometalinter ./... $args

echo "Double checking spelling..."
misspell -error src *.md
