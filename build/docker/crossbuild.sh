#!/bin/sh
set -ex

case $GOARCH in
  arm64)
    curl -s https://musl.cc/aarch64-linux-musl-cross.tgz | tar xz --strip-components=1 -C /usr
    export CC=aarch64-linux-musl-gcc
    ;;

  amd64)
    curl -s https://musl.cc/x86_64-linux-musl-cross.tgz | tar xz --strip-components=1 -C /usr
    export CC=x86_64-linux-musl-gcc
    ;;

  386)
    curl -s https://musl.cc/i686-linux-musl-cross.tgz | tar xz --strip-components=1 -C /usr
    export CC=i686-linux-musl-gcc
    ;;

  arm)
    curl -s https://musl.cc/armv7l-linux-musleabihf-cross.tgz | tar xz --strip-components=1 -C /usr
    export CC=armv7l-linux-musleabihf-gcc
    ;;

  *)
    echo "Unknown GOARCH=${GOARCH}"
    exit 1
    ;;
esac

go env
CGO_ENABLED=1 go build -v -ldflags="-linkmode external -extldflags -static ${FLAGS}" -trimpath -o /out/ ./cmd/...