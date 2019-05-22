#! /bin/bash

# Builds, tests and lints dendrite, and should be run before pushing commits

set -eu

echo "Checking that it builds"
go build

# Check that all the packages can build.
# When `go build` is given multiple packages it won't output anything, and just
# checks that everything builds.
echo "Double checking it builds..."
go build ./cmd/...

./scripts/find-lint.sh

echo "Testing..."
go test ./...
