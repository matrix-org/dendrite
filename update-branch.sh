#! /usr/bin/env sh

#
# This is because matrix-org doesn't allow anonymous contributors.
# You can run it as a curlpipe if you're brave.
#

if [ "$(pwd)" != "$HOME/go/src/github.com/matrix-org/dendrite" ]; then
    if [ ! -d "$HOME/go/src/github.com/matrix-org/dendrite" ]; then
        git clone https://github.com/matrix-org/dendrite "$HOME/go/src/github.com/matrix-org/dendrite"
        cd "$HOME/go/src/github.com/matrix-org/dendrite"
        git remote add idk https://github.com/eyedeekay/dendrite
        git pull idk i2p-demo
    fi
    cd "$HOME/go/src/github.com/matrix-org/dendrite"
    git checkout i2p-demo
fi

echo "Pulling in changes from matrix-org main"
git pull origin main

echo "Pulling in changes from eyedeekay i2p-demo"
git pull idk i2p-demo

echo "Generating binary"
go build -o bin/dendrite-demo-i2p ./cmd/dendrite-demo-i2p