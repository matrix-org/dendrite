#!/usr/bin/env bash

# This script is used to extract the version string from the package.json file
echo "$(cat package.json | grep version | head -1 | awk -F: '{ print $2 }' | sed 's/[",]//g' | tr -d '[[:space:]]')" 