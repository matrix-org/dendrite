FROM golang:1.16-stretch as build
RUN apt-get update && apt-get install -y sqlite3
WORKDIR /build

# Utilise Docker caching when downloading dependencies, this stops us needlessly
# downloading dependencies every time.
COPY go.mod .
COPY go.sum .
RUN go mod download

COPY . .
RUN go build ./cmd/dendrite-monolith-server
RUN go build ./cmd/generate-keys
RUN go build ./cmd/generate-config
RUN ./generate-keys --private-key matrix_key.pem

ENV SERVER_NAME=localhost
EXPOSE 8008 8448

# At runtime, generate TLS cert based on the CA now mounted at /ca
# At runtime, replace the SERVER_NAME with what we are told
CMD ./generate-keys --server $SERVER_NAME --tls-cert server.crt --tls-key server.key --tls-authority-cert /ca/ca.crt --tls-authority-key /ca/ca.key && \
 ./generate-config -server $SERVER_NAME --ci > dendrite.yaml && \
 cp /ca/ca.crt /usr/local/share/ca-certificates/ && update-ca-certificates && \
 ./dendrite-monolith-server --tls-cert server.crt --tls-key server.key --config dendrite.yaml
