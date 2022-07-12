#!/bin/bash -eu

cd $(dirname "$0")

../bin/dendrite-monolith-server \
    --tls-cert server.crt \
    --tls-key server.key \
    --config dendrite.yaml \
    --really-enable-open-registration