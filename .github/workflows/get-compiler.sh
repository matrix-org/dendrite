#!/bin/bash
set -eu

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
        echo "x86_64-linux-gnu-gcc"
        ;;
    "arm64")
        echo "aarch64-linux-gnu-gcc"
        ;;
    "arm")
        echo "arm-linux-gnueabihf-gcc"
        ;;
    *)
        echo "gcc" # Send us a pull request if RISC-V ever takes off
        ;;
    esac
}

# Given a GOARCH target, return a list of Ubuntu packages needed to compile for that target.
function get_pkgs() {
    case "$GOARCH" in
    "arm64")
        echo "gcc-aarch64-linux-gnu libc6-dev-arm64-cross"
        ;;
    "arm")
        echo "gcc-arm-linux-gnueabihf libc6-dev-armhf-cross"
        ;;
    "amd64" | *)
        # We (currently) don't need to install more packages on amd64.
        ;;
    esac
}

case "$1" in
"pkgs")
    get_pkgs
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