#
# base installs required dependencies and runs go mod download to cache dependencies
#
ARG BUILDPLATFORM=${BUILDPLATFORM}
FROM --platform=$BUILDPLATFORM docker.io/golang:1.20-alpine AS base
RUN apk --update --no-cache add bash build-base curl

#
# build creates all needed binaries
#
FROM --platform=$BUILDPLATFORM base AS build
WORKDIR /src
ARG TARGETOS
ARG TARGETARCH
ARG FLAGS

# Mount volumes using the -v flag instead of --mount to avoid requiring BuildKit which is not easily supported using cloudbuild.
COPY . .
RUN mkdir -p /root/.cache/go-build && \
    mkdir -p /go/pkg/mod
VOLUME /root/.cache/go-build
VOLUME /go/pkg/mod

# Run the build command in multiple RUN commands
RUN USERARCH=`go env GOARCH`
ENV GOARCH="$TARGETARCH"
ENV GOOS="linux"
RUN CGO_ENABLED=$([ "$TARGETARCH" = "$USERARCH" ] && echo "1" || echo "0")
RUN go build -v -ldflags="${FLAGS}" -trimpath -o /out/ ./cmd/...

#
# Builds the Dendrite image containing all required binaries
#
FROM alpine:latest
RUN apk --update --no-cache add curl
LABEL org.opencontainers.image.title="Dendrite"
LABEL org.opencontainers.image.description="Next-generation Matrix homeserver written in Go"
LABEL org.opencontainers.image.source="https://github.com/matrix-org/dendrite"
LABEL org.opencontainers.image.licenses="Apache-2.0"
LABEL org.opencontainers.image.vendor="The Matrix.org Foundation C.I.C."
LABEL org.opencontainers.image.documentation="https://matrix-org.github.io/dendrite/"

COPY --from=build /out/create-account /usr/bin/create-account
COPY --from=build /out/generate-config /usr/bin/generate-config
COPY --from=build /out/generate-keys /usr/bin/generate-keys
COPY --from=build /out/dendrite /usr/bin/dendrite

VOLUME /etc/dendrite
WORKDIR /etc/dendrite

ENTRYPOINT ["/usr/bin/dendrite"]
EXPOSE 8008 8448