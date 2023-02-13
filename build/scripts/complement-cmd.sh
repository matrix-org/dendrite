#!/bin/bash -e

# This script is intended to be used inside a docker container for Complement

if [[ "${COVER}" -eq 1 ]]; then
  echo "Running with coverage"
  exec /dendrite/dendrite-monolith-server-cover \
    --really-enable-open-registration \
    --tls-cert server.crt \
    --tls-key server.key \
    --config dendrite.yaml \
    --test.coverprofile=complementcover.log
else
  echo "Not running with coverage"
  exec /dendrite/dendrite \
    --really-enable-open-registration \
    --tls-cert server.crt \
    --tls-key server.key \
    --config dendrite.yaml
fi
