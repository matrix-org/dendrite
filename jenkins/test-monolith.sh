#!/bin/bash

set -eux

cd `dirname $0`/..

: ${WORKSPACE:="$(pwd)"}
export WORKSPACE

# remove any detritus from last time
rm -f sytest/server-*/*.log sytest/results.tap

./jenkins/prepare-dendrite.sh

if [ ! -d "sytest" ]; then
    git clone https://github.com/matrix-org/sytest.git --depth 1 --branch dendrite
else
    git -C sytest fetch --depth 1 origin dendrite
    git -C sytest reset --hard FETCH_HEAD
fi

./sytest/jenkins/prep_sytest_for_postgres.sh

./sytest/jenkins/install_and_run.sh \
    -I Dendrite::Monolith \
    --dendrite-binary-directory $WORKSPACE/bin
