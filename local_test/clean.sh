#!/bin/bash -Eeu

cd $(dirname "$0")

source vars.env

echo "Cleaning ${TEST_DIR}"

rm -rf ${TEST_DIR} 
