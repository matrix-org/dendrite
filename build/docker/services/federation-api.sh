#!/bin/bash

bash ./docker/build.sh

./bin/dendrite-federation-api-server --config dendrite.yaml
