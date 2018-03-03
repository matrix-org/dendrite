#!/bin/bash

gb build

# Generate the keys if they don't already exist.
if [ ! -f server.key ] || [ ! -f server.crt ] || [ ! -f matrix_key.pem ]; then
    echo "Generating keys ..."

    rm -f server.key server.crt matrix_key.pem

    test -f server.key || openssl req -x509 -newkey rsa:4096 \
                        -keyout server.key \
                        -out server.crt \
                        -days 3650 -nodes \
                        -subj /CN=localhost 

    test -f matrix_key.pem || /build/bin/generate-keys -private-key matrix_key.pem
fi
