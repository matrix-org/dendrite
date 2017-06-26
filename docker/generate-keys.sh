#!/bin/bash

set -e

# generate self-signed SSL cert (unlike synapse, dendrite doesn't autogen yet)
# N.B. to specify the right CN if needed
test -f certs/server.key || openssl req -x509 -newkey rsa:4096 -keyout certs/server.key -out certs/server.crt -days 3650 -nodes -subj /CN=$(hostname)

# generate ed25519 signing key
test -f certs/matrix_key.pem || python > certs/matrix_key.pem <<EOF
import base64;
r = lambda n: base64.b64encode(open("/dev/urandom", "rb").read(n)).decode("utf8");
print "-----BEGIN MATRIX PRIVATE KEY-----"
print "Key-ID:", "ed25519:" + r(3).rstrip("=")
print r(32)
print "-----END MATRIX PRIVATE KEY-----"
EOF
