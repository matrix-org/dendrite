#!/bin/bash

bash ./docker/build.sh

./bin/dendrite-edu-server --config=dendrite.yaml
