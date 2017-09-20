#!/bin/bash

set -eux

cd `dirname $0`/..

: ${WORKSPACE:="$(pwd)"}
export WORKSPACE

# remove any detritus from last time
rm -f sytest/server-*/*.log sytest/results.tap

./jenkins/prepare-dendrite.sh

if [ ! -d "sytest" ]; then
    git clone https://github.com/matrix-org/sytest.git --depth 1 --branch master
fi

# Jenkins may have supplied us with the name of the branch in the
# environment. Otherwise we will have to guess based on the current
# commit.
: ${GIT_BRANCH:="origin/$(git rev-parse --abbrev-ref HEAD)"}

git -C sytest fetch --depth 1 origin "${GIT_BRANCH}" || {
    echo >&2 "No ref ${GIT_BRANCH} found, falling back to develop"
    git -C sytest fetch --depth 1 origin develop
}

git -C sytest reset --hard FETCH_HEAD

./sytest/jenkins/prep_sytest_for_postgres.sh

./sytest/jenkins/install_and_run.sh \
    -I Dendrite::Monolith \
    --dendrite-binary-directory "$WORKSPACE/bin" || true
