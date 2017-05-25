#! /bin/bash

set -eu

# Check that the servers build (this is done explicitly because `gb build` can silently fail (exit 0) and then we'd test a stale binary)
gb build github.com/matrix-org/dendrite/cmd/dendrite-room-server
gb build github.com/matrix-org/dendrite/cmd/roomserver-integration-tests
gb build github.com/matrix-org/dendrite/cmd/dendrite-sync-api-server
gb build github.com/matrix-org/dendrite/cmd/syncserver-integration-tests
gb build github.com/matrix-org/dendrite/cmd/create-account

# Run the pre commit hooks
./hooks/pre-commit

# Run the integration tests
bin/roomserver-integration-tests
bin/syncserver-integration-tests
