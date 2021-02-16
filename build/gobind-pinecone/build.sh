#!/bin/sh

TARGET=""

while getopts "ai" option
do
    case "$option"
    in
    a) gomobile bind -v -target android github.com/matrix-org/dendrite/build/gobind-pinecone ;;
    i) gomobile bind -v -target ios github.com/matrix-org/dendrite/build/gobind-pinecone ;;
    *) echo "No target specified, specify -a or -i"; exit 1 ;;
    esac
done