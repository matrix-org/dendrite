#! /bin/bash

# Builds, tests and lints dendrite, and should be run before pushing commits

set -eu

export GOPATH="$(pwd):$(pwd)/vendor"
export PATH="$PATH:$(pwd)/vendor/bin:$(pwd)/bin"

if [ "${TEST_SUITE-}" != "lint" ]; then
    echo "Checking that it builds"
    gb build

    # Check that all the packages can build.
    # When `go build` is given multiple packages it won't output anything, and just
    # checks that everything builds. This seems to do a better job of handling
    # missing imports than `gb build` does.
    echo "Double checking it builds..."
    go build github.com/matrix-org/dendrite/cmd/...
fi

if [ "${TEST_SUITE-lint}" == "lint" ]; then
    ./scripts/find-lint.sh
fi

echo "Double checking spelling..."
misspell -error src *.md

if [ "${TEST_SUITE-unit-test}" == "unit-test" ]; then
    echo "Testing..."
    gb test
fi
