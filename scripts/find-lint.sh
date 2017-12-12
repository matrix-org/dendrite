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

export GOPATH="$(pwd):$(pwd)/vendor"

# prefer the versions of gometalinter and the linters that we install
# to anythign that ends up on the PATH.
export PATH="$(pwd)/bin:$PATH"

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
gb build github.com/alecthomas/gometalinter/
gometalinter --config=linter.json ./... --install

echo "Looking for lint..."
gometalinter ./... $args

echo "Double checking spelling..."
misspell -error src *.md
