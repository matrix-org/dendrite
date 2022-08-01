#!/bin/bash -Eeu

cd $(dirname "$0")

source vars.env

echo "Cleaning data from ${TEST_DIR}"

for ((I = 0; I < ${NUM_NODES}; I++))
do
  NODE_DIR="${TEST_DIR}/node${I}"
  echo "Cleaning node ${I} at ${NODE_DIR}"
  cd ${NODE_DIR}
  rm *.db
  rm logs/*
  rm -rf jetstream
done
