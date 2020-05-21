#!/bin/bash

bash ./docker/build.sh

./bin/dendrite-sync-api-server --config=dendrite.yaml
