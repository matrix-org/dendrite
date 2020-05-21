#!/bin/bash

bash ./docker/build.sh

./bin/client-api-proxy --bind-address ":8008" \
    --client-api-server-url "http://client_api:7771" \
    --sync-api-server-url "http://sync_api:7773" \
    --media-api-server-url "http://media_api:7774" \
    --public-rooms-api-server-url "http://public_rooms_api:7775" \
