#!/bin/sh

# Kick off postgres
export POSTGRES_DB=dendrite
export POSTGRES_USER=dendrite
export POSTGRES_PASSWORD=itsasecret
./usr/local/bin/docker-entrypoint.sh postgres &

# Wait for postgres to be ready
sleep 5
while ! pg_isready;
do
  sleep 1
done

# Start nginx
echo "Starting Riot"
nginx

echo "Postgres and Riot are now running!"
echo "Please start Dendrite using:"
echo ""
echo "  GO111MODULE=on go get github.com/matrix-org/dendrite-p2p-demo"
echo "  \`go env GOPATH\`/bin/dendrite-p2p-demo -d 8081"
echo ""
echo "https://matrix.org/~matthew/p2p-demo.html"

# Wait forever
exec tail -f /dev/null
