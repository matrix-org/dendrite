#!/bin/sh

#!/bin/sh

TARGET=""

while getopts "ai" option
do
    case "$option"
    in
    a) TARGET="android";;
    i) TARGET="ios";;
    esac
done

if [[ $TARGET = "" ]];
then
    echo "No target specified, specify -a or -i"
    exit 1
fi

gomobile bind -v \
    -target $TARGET \
    -ldflags "-X github.com/yggdrasil-network/yggdrasil-go/src/version.buildName=dendrite" \
    github.com/matrix-org/dendrite/build/gobind-pinecone