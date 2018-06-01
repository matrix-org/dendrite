#!/bin/sh

set -eux

cd `dirname $0`

# -u so that if this is run on a dev box, we get the latest deps, as
# we do on travis.

go get -u \
   github.com/alecthomas/gometalinter \
   golang.org/x/crypto/ed25519 \
   github.com/matrix-org/util \
   github.com/matrix-org/gomatrix \
   github.com/tidwall/gjson \
   github.com/tidwall/sjson \
   github.com/pkg/errors \
   gopkg.in/yaml.v2 \
   gopkg.in/macaroon.v2 \

./hooks/pre-commit
