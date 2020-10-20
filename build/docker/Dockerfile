FROM docker.io/golang:1.15-alpine AS builder

RUN apk --update --no-cache add bash build-base

WORKDIR /build

COPY . /build

RUN mkdir -p bin
RUN sh ./build.sh