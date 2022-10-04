#!/bin/sh
set -e

# In order to cross-compile with the multi-stage Docker builds, we need to
# ensure that the suitable toolchain for cross-compiling is installed. Since
# the images are Alpine-based, we will use musl. Download and install the
# toolchain inside the build container.

USERARCH=`go env GOARCH`
GOARCH="$TARGETARCH"
GOOS="linux"

echo "Target arch: $TARGETARCH"
echo "User arch:   $USERARCH"

if [ "$TARGETARCH" != "$USERARCH" ]; then
    echo "Cross compile"
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

    s390x)
        curl -s https://musl.cc/s390x-linux-musl-cross.tgz | tar xz --strip-components=1 -C /usr
        export CC=s390x-linux-musl-gcc
        ;;

    ppc64le)
        curl -s https://musl.cc/powerpc64le-linux-musl-cross.tgz | tar xz --strip-components=1 -C /usr
        export CC=powerpc64le-linux-musl-gcc
        ;;

    riscv64)
        curl -s https://musl.cc/riscv64-linux-musl-cross.tgz | tar xz --strip-components=1 -C /usr
        export CC=riscv64-linux-musl-gcc
        ;;

    *)
        echo "Unsupported GOARCH=${GOARCH}"
        exit 1
        ;;
    esac
else
    echo "Native compile"
fi

# Output the go environment just in case it is useful for debugging.
go env

# Build Dendrite and tools, statically linking them.
CGO_ENABLED=1 go build -v -ldflags="-linkmode external -extldflags -static ${FLAGS}" -trimpath -o /out/ ./cmd/...
