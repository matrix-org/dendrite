#!/bin/bash -e

# This script is intended to be used inside a docker container for Complement

export GOCOVERDIR=/tmp/covdatafiles
mkdir -p "${GOCOVERDIR}"
if [[ "${COVER}" -eq 1 ]]; then
  echo "Running with coverage"
  exec /dendrite/dendrite-cover \
    --really-enable-open-registration \
    --tls-cert server.crt \
    --tls-key server.key \
    --config dendrite.yaml
else
  echo "Not running with coverage"
  exec /dendrite/dendrite \
    --really-enable-open-registration \
    --tls-cert server.crt \
    --tls-key server.key \
    --config dendrite.yaml
fi
