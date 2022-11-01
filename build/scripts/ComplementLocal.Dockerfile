#syntax=docker/dockerfile:1.2

# A local development Complement dockerfile, to be used with host mounts
# /cache -> Contains the entire dendrite code at Dockerfile build time. Builds binaries but only keeps the generate-* ones. Pre-compilation saves time.
# /dendrite -> Host-mounted sources
# /runtime -> Binaries and config go here and are run at runtime
# At runtime, dendrite is built from /dendrite and run in /runtime.
#
# Use these mounts to make use of this dockerfile:
# COMPLEMENT_HOST_MOUNTS='/your/local/dendrite:/dendrite:ro;/your/go/path:/go:ro'
FROM golang:1.18-stretch
RUN apt-get update && apt-get install -y sqlite3

ENV SERVER_NAME=localhost
EXPOSE 8008 8448

WORKDIR /runtime
# This script compiles Dendrite for us.
RUN echo '\
    #!/bin/bash -eux \n\
    if test -f "/runtime/dendrite-monolith-server"; then \n\
    echo "Skipping compilation; binaries exist" \n\
    exit 0 \n\
    fi \n\
    cd /dendrite \n\
    go build -v -o /runtime /dendrite/cmd/dendrite-monolith-server \n\
    ' > compile.sh && chmod +x compile.sh

# This script runs Dendrite for us. Must be run in the /runtime directory.
RUN echo '\
    #!/bin/bash -eu \n\
    ./generate-keys --private-key matrix_key.pem \n\
    ./generate-keys -keysize 1024 --server $SERVER_NAME --tls-cert server.crt --tls-key server.key --tls-authority-cert /complement/ca/ca.crt --tls-authority-key /complement/ca/ca.key \n\
    ./generate-config -server $SERVER_NAME --ci > dendrite.yaml \n\
    cp /complement/ca/ca.crt /usr/local/share/ca-certificates/ && update-ca-certificates \n\
    exec ./dendrite-monolith-server --really-enable-open-registration --tls-cert server.crt --tls-key server.key --config dendrite.yaml \n\
    ' > run.sh && chmod +x run.sh


WORKDIR /cache
# Build the monolith in /cache - we won't actually use this but will rely on build artifacts to speed
# up the real compilation. Build the generate-* binaries in the true /runtime locations.
# If the generate-* source is changed, this dockerfile needs re-running.
RUN --mount=target=. \
    --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -o /runtime ./cmd/generate-config && \
    go build -o /runtime ./cmd/generate-keys


WORKDIR /runtime
CMD /runtime/compile.sh && exec /runtime/run.sh
