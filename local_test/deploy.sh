#!/bin/bash -Eeu

cd $(dirname "$0")

./stop_all.sh
./clean.sh

source vars.env

echo "Deploying to ${TEST_DIR}"

for ((i = 0; i < ${NUM_NODES}; i++))
do
  NODE_DIR="${TEST_DIR}/node${i}"
  echo "Deploying node ${i} to ${NODE_DIR}"
  mkdir -p ${NODE_DIR}
  cp dendrite.yaml ${NODE_DIR}
  ../bin/generate-keys --private-key ${NODE_DIR}/matrix_key.pem
  ../bin/generate-keys --tls-cert ${NODE_DIR}/server.crt --tls-key ${NODE_DIR}/server.key
done
