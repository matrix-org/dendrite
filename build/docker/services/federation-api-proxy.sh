#!/bin/bash

bash ./docker/build.sh

./bin/federation-api-proxy --bind-address ":8448" \
    --federation-api-url "http://federation_api_server:7772" \
    --media-api-server-url "http://media_api:7774" \
