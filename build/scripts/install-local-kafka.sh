#! /bin/bash

# Downloads, installs and runs a kafka instance

set -eu

cd `dirname $0`/..

mkdir -p .downloads

KAFKA_URL=http://archive.apache.org/dist/kafka/2.1.0/kafka_2.11-2.1.0.tgz

# Only download the kafka if it isn't already downloaded.
test -f .downloads/kafka.tgz || wget $KAFKA_URL -O .downloads/kafka.tgz
# Unpack the kafka over the top of any existing installation
mkdir -p kafka && tar xzf .downloads/kafka.tgz -C kafka --strip-components 1
# Start the zookeeper running in the background.
# By default the zookeeper listens on localhost:2181
kafka/bin/zookeeper-server-start.sh -daemon kafka/config/zookeeper.properties
# Enable topic deletion so that the integration tests can create a fresh topic
# for each test run.
echo -e "\n\ndelete.topic.enable=true" >> kafka/config/server.properties
# Start the kafka server running in the background.
# By default the kafka listens on localhost:9092
kafka/bin/kafka-server-start.sh -daemon kafka/config/server.properties
