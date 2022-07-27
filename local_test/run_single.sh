#!/bin/bash -Eeu

cd $(dirname "$0")

source vars.env

echo "Running from ${TEST_DIR}"

I=$1

SCRIPT_DIR=$PWD

NODE_DIR="${TEST_DIR}/node${I}"

echo "Running node ${I} from ${NODE_DIR}"

cd ${NODE_DIR}
${SCRIPT_DIR}/../bin/dendrite-monolith-server \
  --tls-cert server.crt \
  --tls-key server.key \
  --config dendrite.yaml \
  --really-enable-open-registration \
  --http-bind-address ":$((8008 + $I))" \
  --https-bind-address ":$((8448 + $I))"
