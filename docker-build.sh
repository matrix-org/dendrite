#!/bin/bash

set -e

GOOS=linux GOARCH=amd64 gb build

mkdir -p docker/bin
cp bin/*linux-amd64 docker/bin/

cd docker

for cli in {client,federation}-api-proxy dendrite-{{client,federation,media,sync}-api,room}-server; do
    dockerfile=Dockerfile.$cli
    cat <<EOF > $dockerfile
FROM scratch
COPY bin/$cli-linux-amd64 $cli
ENTRYPOINT ["/$cli"]
EOF
    docker build -t $cli -f $dockerfile .
    rm $dockerfile
done
