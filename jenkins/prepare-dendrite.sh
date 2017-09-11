#!/bin/bash
#
# build the dendrite binaries into ./bin

cd `dirname $0`/..

set -eux

export GOPATH=`pwd`/.gopath
export PATH="${GOPATH}/bin:$PATH"

go get github.com/constabulary/gb/...
gb build
