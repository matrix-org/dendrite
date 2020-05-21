#!/bin/bash

bash ./docker/build.sh

./bin/dendrite-public-rooms-api-server --config dendrite.yaml
