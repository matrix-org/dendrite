#!/bin/bash

set -euo pipefail

if [ -z "${1}" ]; then
  echo "usage: ${0} installDirPath" >&2
  exit 1
fi

BIN_FILE="thrift-1"
TAR_FILE="${BIN_FILE}-$(uname -s | tr '[:upper:]' '[:lower:]')-$(uname -m).tar.gz"
TAR_LOCATION="https://github.com/uber/tchannel-go/releases/download/thrift-v1.0.0-dev/${TAR_FILE}"

mkdir -p "${1}"
cd "${1}"
wget "${TAR_LOCATION}"
tar xzf "${TAR_FILE}"
rm -f "${TAR_FILE}"
mv "${BIN_FILE}" "thrift"
