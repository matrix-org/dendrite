#syntax=docker/dockerfile:1.2

# base installs required dependencies and runs go mod download to cache dependencies
FROM --platform=${BUILDPLATFORM} docker.io/golang:1.18-alpine AS base
RUN apk --update --no-cache add bash build-base

WORKDIR /src
COPY go.* .
RUN go mod download

# build creates all needed binaries
FROM base AS build
ARG TARGETOS
ARG TARGETARCH
RUN --mount=target=. \
    --mount=type=cache,target=/root/.cache/go-build \
    GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -o /out/ ./cmd/...

# dendrite base image; mainly creates a user and switches to it
FROM alpine:3.15 AS dendrite-base
LABEL org.opencontainers.image.description="Next-generation Matrix homeserver written in Go"
LABEL org.opencontainers.image.source="https://github.com/matrix-org/dendrite"
LABEL org.opencontainers.image.licenses="Apache-2.0"
RUN addgroup dendrite && adduser dendrite -G dendrite -u 1337 -D
USER dendrite
WORKDIR /home/dendrite

# polylith image, only contains the polylith binary
FROM dendrite-base AS image-polylith
LABEL org.opencontainers.image.title="Dendrite (Polylith)"

COPY --from=build /out/dendrite-polylith-multi /usr/bin/

ENTRYPOINT ["/usr/bin/dendrite-polylith-multi"]

# monolith image
FROM dendrite-base AS image-monolith
LABEL org.opencontainers.image.title="Dendrite (Monolith)"

COPY --from=build /out/create-account /usr/bin/create-account
COPY --from=build /out/generate-config /usr/bin/generate-config
COPY --from=build /out/generate-keys /usr/bin/generate-keys
COPY --from=build /out/dendrite-monolith-server /usr/bin/dendrite-monolith-server

ENTRYPOINT ["/usr/bin/dendrite-monolith-server"]
EXPOSE 8008 8448

# complement image
FROM base AS image-complement
RUN apk add --no-cache sqlite openssl ca-certificates
COPY --from=build /out/* /usr/bin/
RUN rm /usr/bin/dendrite-polylith-multi

WORKDIR /dendrite
RUN /usr/bin/generate-keys --private-key matrix_key.pem && \
    mkdir /ca && \
    openssl genrsa -out /ca/ca.key 2048 && \
    openssl req -new -x509 -key /ca/ca.key -days 3650 -subj "/C=GB/ST=London/O=matrix.org/CN=Complement CA" -out /ca/ca.crt

ENV SERVER_NAME=localhost
ENV API=0
EXPOSE 8008 8448

# At runtime, generate TLS cert based on the CA now mounted at /ca
# At runtime, replace the SERVER_NAME with what we are told
CMD /usr/bin/generate-keys --server $SERVER_NAME --tls-cert server.crt --tls-key server.key --tls-authority-cert /ca/ca.crt --tls-authority-key /ca/ca.key && \
 /usr/bin/generate-config -server $SERVER_NAME --ci > dendrite.yaml && \
 cp /ca/ca.crt /usr/local/share/ca-certificates/ && update-ca-certificates && \
 /usr/bin/dendrite-monolith-server --tls-cert server.crt --tls-key server.key --config dendrite.yaml -api=${API:-0}