FROM docker.io/golang:1.19-alpine AS base

RUN apk --update --no-cache add bash build-base

WORKDIR /build

#
# The dendrite base image
#
FROM alpine:latest AS dendrite-base
LABEL org.opencontainers.image.title="Dendrite (Monolith)"
LABEL org.opencontainers.image.description="Next-generation Matrix homeserver written in Go"
LABEL org.opencontainers.image.source="https://github.com/matrix-org/dendrite"
LABEL org.opencontainers.image.licenses="Apache-2.0"
LABEL org.opencontainers.image.documentation="https://matrix-org.github.io/dendrite/"
LABEL org.opencontainers.image.vendor="The Matrix.org Foundation C.I.C."

COPY --from=base /build/bin/* /usr/bin/

VOLUME /etc/dendrite
WORKDIR /etc/dendrite

ENTRYPOINT ["/usr/bin/dendrite-monolith-server"]
