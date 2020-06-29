#!/bin/sh

gomobile bind -v \
    -ldflags "-X $github.com/yggdrasil-network/yggdrasil-go/src/version.buildName=riot-ios-p2p" \
    -target ios \
    github.com/matrix-org/dendrite/build/gobind