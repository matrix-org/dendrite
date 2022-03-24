FROM golang:1.16-stretch as build
RUN apt-get update && apt-get install -y sqlite3
WORKDIR /build

# we will dump the binaries and config file to this location to ensure any local untracked files
# that come from the COPY . . file don't contaminate the build
RUN mkdir /dendrite

# Utilise Docker caching when downloading dependencies, this stops us needlessly
# downloading dependencies every time.
COPY go.mod .
COPY go.sum .
RUN go mod download

COPY . .
RUN go build -o /dendrite ./cmd/dendrite-monolith-server
RUN go build -o /dendrite ./cmd/generate-keys
RUN go build -o /dendrite ./cmd/generate-config

WORKDIR /dendrite
RUN ./generate-keys --private-key matrix_key.pem

ENV SERVER_NAME=localhost
ENV API=0
EXPOSE 8008 8448

# At runtime, generate TLS cert based on the CA now mounted at /ca
# At runtime, replace the SERVER_NAME with what we are told
CMD ./generate-keys --server $SERVER_NAME --tls-cert server.crt --tls-key server.key --tls-authority-cert /complement/ca/ca.crt --tls-authority-key /complement/ca/ca.key && \
 ./generate-config -server $SERVER_NAME --ci > dendrite.yaml && \
 cp /complement/ca/ca.crt /usr/local/share/ca-certificates/ && update-ca-certificates && \
 ./dendrite-monolith-server --tls-cert server.crt --tls-key server.key --config dendrite.yaml -api=${API:-0}
