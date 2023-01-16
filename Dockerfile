#syntax=docker/dockerfile:1.2

#
# base installs required dependencies and runs go mod download to cache dependencies
#
FROM --platform=${BUILDPLATFORM} docker.io/golang:1.19-alpine AS base
RUN apk --update --no-cache add bash build-base curl

#
# build creates all needed binaries
#
FROM --platform=${BUILDPLATFORM} base AS build
WORKDIR /src
ARG TARGETOS
ARG TARGETARCH
ARG FLAGS
RUN --mount=target=. \
    --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    USERARCH=`go env GOARCH` \
    GOARCH="$TARGETARCH" \
    GOOS="linux" \
    CGO_ENABLED=$([ "$TARGETARCH" = "$USERARCH" ] && echo "1" || echo "0") \
    go build -v -ldflags="${FLAGS}" -trimpath -o /out/ ./cmd/...

#
# The dendrite base image
#
FROM alpine:latest AS dendrite-base
RUN apk --update --no-cache add curl
LABEL org.opencontainers.image.description="Next-generation Matrix homeserver written in Go"
LABEL org.opencontainers.image.source="https://github.com/matrix-org/dendrite"
LABEL org.opencontainers.image.licenses="Apache-2.0"
LABEL org.opencontainers.image.documentation="https://matrix-org.github.io/dendrite/"
LABEL org.opencontainers.image.vendor="The Matrix.org Foundation C.I.C."

#
# Builds the polylith image and only contains the polylith binary
#
FROM dendrite-base AS polylith
LABEL org.opencontainers.image.title="Dendrite (Polylith)"

COPY --from=build /out/dendrite-polylith-multi /usr/bin/

VOLUME /etc/dendrite
WORKDIR /etc/dendrite

ENTRYPOINT ["/usr/bin/dendrite-polylith-multi"]

#
# Builds the monolith image and contains all required binaries
#
FROM dendrite-base AS monolith
LABEL org.opencontainers.image.title="Dendrite (Monolith)"

COPY --from=build /out/create-account /usr/bin/create-account
COPY --from=build /out/generate-config /usr/bin/generate-config
COPY --from=build /out/generate-keys /usr/bin/generate-keys
COPY --from=build /out/dendrite-monolith-server /usr/bin/dendrite-monolith-server

VOLUME /etc/dendrite
WORKDIR /etc/dendrite

ENTRYPOINT ["/usr/bin/dendrite-monolith-server"]
EXPOSE 8008 8448

