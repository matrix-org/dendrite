#! /bin/bash

# Builds, tests and lints dendrite, and should be run before pushing commits

set -eu

# Check that all the packages can build.
# When `go build` is given multiple packages it won't output anything, and just
# checks that everything builds.
echo "Checking that it builds..."
go build ./cmd/...

./build/scripts/find-lint.sh

echo "Testing..."
go test --race -v ./...
