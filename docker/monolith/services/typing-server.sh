#!/bin/bash

bash ./docker/build.sh

./bin/dendrite-typing-server --config=dendrite.yaml
