#!/bin/bash -Eeu

cd $(dirname "$0")

source vars.env

echo "Running from ${TEST_DIR}"

SCRIPT_DIR=$PWD
PID_FILE=${TEST_DIR}/pids.txt

for ((I = 0; I < ${NUM_NODES}; I++))
do
  NODE_DIR="${TEST_DIR}/node${I}"
  echo "Running node ${I} from ${NODE_DIR}"
  cd ${NODE_DIR}
  mkdir -p logs
  ${SCRIPT_DIR}/../bin/dendrite-monolith-server \
    --tls-cert server.crt \
    --tls-key server.key \
    --config dendrite.yaml \
    --really-enable-open-registration \
    --http-bind-address ":$((8008 + $I))" \
    --https-bind-address ":$((8448 + $I))" \
    >> ${NODE_DIR}/logs/console 2>&1 &
    echo $! >> ${PID_FILE}
done
