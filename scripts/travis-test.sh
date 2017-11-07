#! /bin/bash

# The entry point for travis tests

set -eu

# Tune the GC to use more memory to reduce the number of garbage collections
export GOGC=400
export DENDRITE_LINT_DISABLE_GC=1

# We don't bother to build if we're only checking the linting
if [ "${TEST_SUITE-}" != "lint" ]; then
    # Check that the servers build (this is done explicitly because `gb build` can silently fail (exit 0) and then we'd test a stale binary)
    gb build github.com/matrix-org/dendrite/cmd/dendrite-room-server
    gb build github.com/matrix-org/dendrite/cmd/roomserver-integration-tests
    gb build github.com/matrix-org/dendrite/cmd/dendrite-sync-api-server
    gb build github.com/matrix-org/dendrite/cmd/syncserver-integration-tests
    gb build github.com/matrix-org/dendrite/cmd/create-account
    gb build github.com/matrix-org/dendrite/cmd/dendrite-media-api-server
    gb build github.com/matrix-org/dendrite/cmd/mediaapi-integration-tests
    gb build github.com/matrix-org/dendrite/cmd/client-api-proxy
fi

# Run unit tests and linters
./scripts/build-test-lint.sh

if [ "${TEST_SUITE-unit-test}" == "integ-test" ]; then
    # Run the integration tests
    bin/roomserver-integration-tests
    bin/syncserver-integration-tests
    bin/mediaapi-integration-tests
fi
