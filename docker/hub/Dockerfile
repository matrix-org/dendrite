FROM docker.io/golang:1.13.7-alpine3.11 AS builder

RUN apk --update --no-cache add bash build-base

WORKDIR /build

ADD . /build

RUN mkdir -p bin
RUN sh ./build.sh