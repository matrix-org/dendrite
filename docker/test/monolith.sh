#!/bin/bash

rm -rf build
bash ./docker/build.sh

gb test
