#! /bin/bash

set -eu

# Check that the servers build
gb build github.com/matrix-org/dendrite/cmd/dendrite-roomserver
gb build github.com/matrix-org/dendrite/cmd/roomserver-integration-tests

# Run the pre commit hooks
./hooks/pre-commit

# Run the integration tests
bin/roomserver-integration-tests
