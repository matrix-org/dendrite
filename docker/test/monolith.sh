#!/bin/bash

rm -rf build
bash ./docker/build.sh

TEST_SUITE=unit-test  scripts/travis-test.sh
TEST_SUITE=integ-test scripts/travis-test.sh
TEST_SUITE=lint       scripts/travis-test.sh

