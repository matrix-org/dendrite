#! /bin/bash -eu
# This script is designed for developers who want to test their Dendrite code
# against Complement.
#
# It makes a Dendrite image which represents the current checkout,
# then downloads Complement and runs it with that image.

# Make image
cd `dirname $0`/../..
docker build -t complement-dendrite -f build/scripts/Complement.Dockerfile .

# Download Complement
wget -N https://github.com/matrix-org/complement/archive/master.tar.gz
tar -xzf master.tar.gz

# Run the tests!
cd complement-master
COMPLEMENT_BASE_IMAGE=complement-dendrite:latest go test -v -count=1 ./tests

