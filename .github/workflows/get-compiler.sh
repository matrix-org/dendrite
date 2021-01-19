#!/bin/bash
set -exu

MUSLCC_BASE="https://more.musl.cc/x86_64-linux-musl"

# Given a GOARCH target, return what Docker calls that target.
function get_docker() {
    case "$GOARCH" in
    "amd64")
        echo "linux/amd64"
        ;;
    "arm64")
        echo "linux/arm64/v8"
        ;;
    "arm")
        echo "linux/arm/v7"
        ;;
    *)
        exit 1
        ;;
    esac
}

# Given a GOARCH target, return the GCC for that target.
function get_compiler() {
    case "$GOARCH" in
    "amd64")
        echo "gcc"
        ;;
    "arm64")
        echo "./aarch64-linux-musl-cross/bin/aarch64-linux-musl-gcc"
        ;;
    "arm")
        echo "./arm-linux-musleabi-cross/bin/arm-linux-musleabi-gcc"
        ;;
    *)
        exit 1 # Send us a pull request if RISC-V ever takes off
        ;;
    esac
}

function download_musl() {
    case "$GOARCH" in
    "arm64")
        curl "${MUSLCC_BASE}/aarch64-linux-musl-cross.tgz" -o musl.tgz
        tar xzf musl.tgz
        ;;
    "arm")
        curl "${MUSLCC_BASE}/arm-linux-musleabi-cross.tgz" -o musl.tgz
        tar xzf musl.tgz
        ;;
    "amd64")
        echo "nothing to do"
        ;;
    esac
}

case "$1" in
"pkgs")
    download_musl
    ;;
"ccomp")
    get_compiler
    ;;
"docker")
    get_docker
    ;;
*)
    exit 1
    ;;
esac
