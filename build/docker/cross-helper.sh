#!/bin/bash
set -eu

# Utility script for getting and using cross-compilation toolchains in Docker.
# You really shouldn't run this outside of a container, though there's no reason you can't.

# $1 is used for arch
function install_musl() {
    rm musl.tgz
    cd ${1}-cross
    rm -f $(find . -name "ld-musl-*.so.1")
    rm usr
    # error supression!
    rsync --ignore-errors -rLaq . / || true
    cd ..
    rm -rf ${1}-cross
}

function get_triple() {
    case "${1}" in
    "arm64")
        echo "aarch64-linux-musl"
        ;;
    "arm")
        echo "arm-linux-musleabi"
        ;;
    "amd64")
        echo "x86_64-linux-musl"
        ;;
    *)
        exit 1
        ;;
    esac
}

arch=$(go env GOARCH)

case "${1}" in
"dl-compiler")
    if [[ "${arch}" == "$(go env GOHOSTARCH)" ]]; then
        echo "not cross-compiling, nothing to do"
        exit 0
    fi
    MUSLCC_BASE="https://more.musl.cc/${HOST_TRIPLE}"
    target="$(get_triple ${arch})"
    curl "${MUSLCC_BASE}/${target}-cross.tgz" -o musl.tgz
    tar xzf musl.tgz
    install_musl ${target}
    ;;
"cc")
    if [[ ${arch} == "$(go env GOHOSTARCH)" ]]; then
        echo "gcc"
    else
        echo "$(get_triple $(go env GOARCH))-gcc"
    fi
    ;;
*)
    exit 1
esac
