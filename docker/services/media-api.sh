#!/bin/bash

bash ./docker/build.sh

./bin/dendrite-media-api-server --config dendrite.yaml
