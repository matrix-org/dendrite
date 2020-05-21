#!/bin/bash

bash ./docker/build.sh

./bin/dendrite-federation-sender-server --config dendrite.yaml
