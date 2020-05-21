#!/bin/bash

bash ./docker/build.sh

./bin/dendrite-monolith-server --tls-cert=server.crt --tls-key=server.key $@
