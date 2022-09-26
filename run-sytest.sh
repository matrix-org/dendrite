#!/usr/bin/env bash
#
# Runs SyTest either from Docker Hub, or from ../sytest. If it's run
# locally, the Docker image is rebuilt first.
#
# Logs are stored in ../sytestout/logs.

set -e
set -o pipefail

main() {
    local tag=buster
    local base_image=debian:$tag
    local runargs=()

    cd "$(dirname "$0")"

    if [ -d ../sytest ]; then
        local tmpdir
        tmpdir="$(mktemp -d -t run-systest.XXXXXXXXXX)"
        trap "rm -r '$tmpdir'" EXIT

        if [ -z "$DISABLE_BUILDING_SYTEST" ]; then
            echo "Re-building ../sytest Docker images..."

            local status
            (
                cd ../sytest

                docker build -f docker/base.Dockerfile --build-arg BASE_IMAGE="$base_image" --tag matrixdotorg/sytest:"$tag" .
                docker build -f docker/dendrite.Dockerfile --build-arg SYTEST_IMAGE_TAG="$tag" --tag matrixdotorg/sytest-dendrite:latest .
            ) &>"$tmpdir/buildlog" || status=$?
            if (( status != 0 )); then
                # Docker is very verbose, and we don't really care about
                # building SyTest. So we accumulate and only output on
                # failure.
                cat "$tmpdir/buildlog" >&2
                return $status
            fi
        fi

        runargs+=( -v "$PWD/../sytest:/sytest:ro" )
    fi
    if [ -n "$SYTEST_POSTGRES" ]; then
        runargs+=( -e POSTGRES=1 )
    fi

    local sytestout=$PWD/../sytestout
    mkdir -p "$sytestout"/{logs,cache/go-build,cache/go-pkg}
    docker run \
           --rm \
           --name "sytest-dendrite-${LOGNAME}" \
           -e LOGS_USER=$(id -u) \
           -e LOGS_GROUP=$(id -g) \
           -v "$PWD:/src/:ro" \
           -v "$sytestout/logs:/logs/" \
           -v "$sytestout/cache/go-build:/root/.cache/go-build" \
           -v "$sytestout/cache/go-pkg:/gopath/pkg" \
           "${runargs[@]}" \
           matrixdotorg/sytest-dendrite:latest "$@"
}

main "$@"
