#!/bin/sh

ARGS="-v ./cmd/..."

usage() {
  echo "Usage: $0 [-r]" 1>&2
  echo
  echo "-r"
  echo "  Build with race condition detection"
  exit 1
}

while getopts ":r" o; do
    case "${o}" in
        r)
            # Turn on race condition detection
            ARGS="-race $ARGS"
            ;;
        *)
            usage
            ;;
    esac
done

GOBIN=$PWD/`dirname $0`/bin go install $ARGS
