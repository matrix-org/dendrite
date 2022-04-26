FROM golang:1.16-stretch as build
RUN apt-get update && apt-get install -y postgresql
WORKDIR /build

# No password when connecting over localhost
RUN sed -i "s%127.0.0.1/32            md5%127.0.0.1/32            trust%g" /etc/postgresql/9.6/main/pg_hba.conf && \
    # Bump up max conns for moar concurrency
    sed -i 's/max_connections = 100/max_connections = 2000/g' /etc/postgresql/9.6/main/postgresql.conf

# This entry script starts postgres, waits for it to be up then starts dendrite
RUN echo '\
#!/bin/bash -eu \n\
pg_lsclusters \n\
pg_ctlcluster 9.6 main start \n\
 \n\
until pg_isready \n\
do \n\
  echo "Waiting for postgres"; \n\
  sleep 1; \n\
done \n\
' > run_postgres.sh && chmod +x run_postgres.sh

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
CMD /build/run_postgres.sh && ./generate-keys --server $SERVER_NAME --tls-cert server.crt --tls-key server.key --tls-authority-cert /complement/ca/ca.crt --tls-authority-key /complement/ca/ca.key && \
 ./generate-config -server $SERVER_NAME --ci > dendrite.yaml && \
 # Replace the connection string with a single postgres DB, using user/db = 'postgres' and no password, bump max_conns
 sed -i "s%connection_string:.*$%connection_string: postgresql://postgres@localhost/postgres?sslmode=disable%g" dendrite.yaml && \
 sed -i 's/max_open_conns:.*$/max_open_conns: 100/g' dendrite.yaml && \
 cp /complement/ca/ca.crt /usr/local/share/ca-certificates/ && update-ca-certificates && \
 ./dendrite-monolith-server --tls-cert server.crt --tls-key server.key --config dendrite.yaml -api=${API:-0}