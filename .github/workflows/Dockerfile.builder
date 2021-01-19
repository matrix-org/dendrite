FROM docker.io/golang:alpine AS builder
ARG GOARCH=amd64

RUN apk --update --no-cache add bash build-base curl

WORKDIR /build

COPY . /build

RUN bash /build/.github/workflows/get-compiler.sh pkgs

# Build!
RUN CC="$(/build/.github/workflows/get-compiler.sh ccomp)" bash /build/build.sh
