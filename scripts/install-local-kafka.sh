# /bin/bash

# Downloads, installs and runs a kafka instance

set -eu

# The mirror to download kafka from is picked from the list of mirrors at
# https://www.apache.org/dyn/closer.cgi?path=/kafka/0.10.2.0/kafka_2.11-0.11.0.2.tgz
# TODO: Check the signature since we are downloading over HTTP.
MIRROR=http://apache.mirror.anlx.net/kafka/0.11.0.2/kafka_2.11-0.11.0.2.tgz

# Only download the kafka if it isn't already downloaded.
test -f kafka.tgz || wget $MIRROR -O kafka.tgz
# Unpack the kafka over the top of any existing installation
mkdir -p kafka && tar xzf kafka.tgz -C kafka --strip-components 1
# Start the zookeeper running in the background.
# By default the zookeeper listens on localhost:2181
kafka/bin/zookeeper-server-start.sh -daemon kafka/config/zookeeper.properties
# Enable topic deletion so that the integration tests can create a fresh topic
# for each test run.
echo "delete.topic.enable=true" >> kafka/config/server.properties
# Start the kafka server running in the background.
# By default the kafka listens on localhost:9092
kafka/bin/kafka-server-start.sh -daemon kafka/config/server.properties
