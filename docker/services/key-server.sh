#!/bin/bash

bash ./docker/build.sh

./bin/dendrite-key-server --config dendrite.yaml
