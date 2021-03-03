# The binary to build the image around (monolith or polylith).
ARG DENDRITE_BINARY

FROM --platform=$BUILDPLATFORM docker.io/golang:alpine AS base
ARG TARGETARCH
ARG DENDRITE_BINARY 
ENV GOARCH=$TARGETARCH
ENV CGO_ENABLED=1
# The host triple of the *builder container.* Will end in musl because Alpine. E.g. aarch64-linux-musl.
# Only relevant when cross-compiling.
ARG HOST_TRIPLE="x86_64-linux-musl"

# Todo: Do we *really* need rsync?
RUN apk --update --no-cache add bash build-base curl rsync

WORKDIR /build
COPY . /build

RUN bash ./build/docker/cross-helper.sh dl-compiler

RUN mkdir -p bin
RUN CC=$(./build/docker/cross-helper.sh cc) go build -trimpath -o bin/ ./cmd/$DENDRITE_BINARY
RUN CC=$(./build/docker/cross-helper.sh cc) go build -trimpath -o bin/ ./cmd/goose
RUN CC=$(./build/docker/cross-helper.sh cc) go build -trimpath -o bin/ ./cmd/create-account
RUN CC=$(./build/docker/cross-helper.sh cc) go build -trimpath -o bin/ ./cmd/generate-keys

FROM alpine:latest
ARG DENDRITE_BINARY
ENV DENDRITE_BINARY=$DENDRITE_BINARY

COPY --from=base /build/bin/* /usr/bin/

VOLUME /etc/dendrite
WORKDIR /etc/dendrite

ENTRYPOINT ["/usr/bin/$DENDRITE_BINARY"]
