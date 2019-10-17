#!/bin/bash

bash ./docker/build.sh

./bin/dendrite-userdirectory-api-server --config dendrite.yaml
