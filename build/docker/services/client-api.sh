#!/bin/bash

bash ./docker/build.sh

./bin/dendrite-client-api-server --config=dendrite.yaml
