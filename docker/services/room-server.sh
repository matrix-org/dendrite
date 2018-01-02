#!/bin/bash

bash ./docker/build.sh

./bin/dendrite-room-server --config=dendrite.yaml
