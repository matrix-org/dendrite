#! /bin/bash

# Builds, tests and lints dendrite, and should be run before pushing commits

set -eu

export GOPATH="$(pwd):$(pwd)/vendor"
export PATH="$PATH:$(pwd)/vendor/bin:$(pwd)/bin"

echo "Checking that it builds"
gb build

# Check that all the packages can build.
# When `go build` is given multiple packages it won't output anything, and just
# checks that everything builds. This seems to do a better job of handling
# missing imports than `gb build` does.
echo "Double checking it builds..."
go build github.com/matrix-org/dendrite/cmd/...

./scripts/find-lint.sh

echo "Double checking spelling..."
misspell -error src *.md

echo "Testing..."
gb test
