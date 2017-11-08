#! /bin/bash

# The entry point for travis tests
#
# TEST_SUITE env var can be set to "lint", "unit-test" or "integ-test", in
# which case only the linting, unit tests or integration tests will be run
# respectively. If not specified or null all tests are run.

set -eu

# Tune the GC to use more memory to reduce the number of garbage collections
export GOGC=400
export DENDRITE_LINT_DISABLE_GC=1

export GOPATH="$(pwd):$(pwd)/vendor"
export PATH="$PATH:$(pwd)/vendor/bin:$(pwd)/bin"

if [ "${TEST_SUITE:-lint}" == "lint" ]; then
    ./scripts/find-lint.sh
fi

if [ "${TEST_SUITE:-unit-test}" == "unit-test" ]; then
    gb test
fi

if [ "${TEST_SUITE:-integ-test}" == "integ-test" ]; then
    gb build

    # Check that all the packages can build.
    # When `go build` is given multiple packages it won't output anything, and just
    # checks that everything builds. This seems to do a better job of handling
    # missing imports than `gb build` does.
    go build github.com/matrix-org/dendrite/cmd/...

    # Check that the servers build (this is done explicitly because `gb build` can silently fail (exit 0) and then we'd test a stale binary)
    gb build github.com/matrix-org/dendrite/cmd/dendrite-room-server
    gb build github.com/matrix-org/dendrite/cmd/roomserver-integration-tests
    gb build github.com/matrix-org/dendrite/cmd/dendrite-sync-api-server
    gb build github.com/matrix-org/dendrite/cmd/syncserver-integration-tests
    gb build github.com/matrix-org/dendrite/cmd/create-account
    gb build github.com/matrix-org/dendrite/cmd/dendrite-media-api-server
    gb build github.com/matrix-org/dendrite/cmd/mediaapi-integration-tests
    gb build github.com/matrix-org/dendrite/cmd/client-api-proxy

    # Create necessary certificates and keys to run dendrite
    time openssl req -x509 -newkey rsa:512 -keyout server.key -out server.crt -days 365 -nodes -subj /CN=localhost
    time ./scripts/install-local-kafka.sh

    # Run the integration tests
    bin/roomserver-integration-tests
    bin/syncserver-integration-tests
    bin/mediaapi-integration-tests
fi
