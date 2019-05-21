#! /bin/bash

# The entry point for travis tests
#
# TEST_SUITE env var can be set to "lint", "unit-test" or "integ-test", in
# which case only the linting, unit tests or integration tests will be run
# respectively. If not specified or null all tests are run.

set -eu

# Tune the GC to use more memory to reduce the number of garbage collections
export GOGC=400
export DENDRITE_LINT_DISABLE_GC=1

export GO111MODULE=on

# starts a travis fold section. The first argument is the name of the fold
# section (which appears on the RHS) and may contain no spaces. Remaining
# arguments are echoed in yellow on the LHS as the header line of the fold
# section.
travis_sections=()
function travis_start {
    name="$1"
    shift
    echo -en "travis_fold:start:$name\r"
    travis_sections+=($name)

    # yellow/bold
    echo -en "\e[33;1m"
    echo "$@"
    # normal
    echo -en "\e[0m"
}

# ends a travis fold section
function travis_end {
    name=${travis_sections[-1]}
    unset 'travis_sections[-1]'
    echo -en "travis_fold:end:$name\r"
}

function kill_kafka {
    echo "killing kafka"
    # sometimes kafka doesn't die on a SIGTERM so we SIGKILL it.
    killall -9 -v java
}

if [ "${TEST_SUITE:-lint}" == "lint" ]; then
    ./scripts/find-lint.sh
fi

if [ "${TEST_SUITE:-unit-test}" == "unit-test" ]; then
    go test ./...
fi

if [ "${TEST_SUITE:-integ-test}" == "integ-test" ]; then
    travis_start go-build "Building dendrite and integ tests"
    go install ./cmd/...
    travis_end
    
    # Check that all the packages can build.
    # When `go build` is given multiple packages it won't output anything, and just
    # checks that everything builds.
    go build ./cmd/...

    # Create necessary certificates and keys to run dendrite
    travis_start certs "Building SSL certs"
    openssl req -x509 -newkey rsa:512 -keyout server.key -out server.crt -days 365 -nodes -subj /CN=localhost
    travis_end

    travis_start kafka "Installing kafka"
    ./scripts/install-local-kafka.sh
    travis_end

    # make sure we kill off zookeeper/kafka on exit, because it stops the
    # travis container being cleaned up (cf
    # https://github.com/travis-ci/travis-ci/issues/8082)
    trap kill_kafka EXIT

    # Run the integration tests
    for i in roomserver syncserver mediaapi; do
        travis_start "$i-integration-tests" "Running integration tests for $i"
        $i-integration-tests
        travis_end
    done
fi
