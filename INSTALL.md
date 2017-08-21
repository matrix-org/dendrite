# Installing Dendrite

Dendrite can be run in one of two configurations:

 * A cluster of individual components, dealing with different aspects of the
   Matrix protocol (see [WIRING.md](./WIRING.md)). Components communicate with
   one another via [Apache Kafka](https://kafka.apache.org).
 
 * A monolith server, in which all components run in the same process. In this
   configuration, Kafka can be replaced with an in-process implementation
   called [naffka](https://github.com/matrix-org/naffka).

## Requirements

 - Go 1.8+
 - Postgres 9.5+
 - For Kafka (optional if using the monolith server):
   - Unix-based system (https://kafka.apache.org/documentation/#os)
   - JDK 1.8+ / OpenJDK 1.8+
   - Apache Kafka 0.10.2+ (see https://github.com/matrix-org/dendrite/blob/master/travis-install-kafka.sh for up-to-date version numbers)


## Setting up a development environment

Assumes Go 1.8 and JDK 1.8 are already installed and are on PATH.

```bash
# Get the code
git clone https://github.com/matrix-org/dendrite
cd dendrite

# Build it
go get github.com/constabulary/gb/...
gb build
```

If using Kafka, install and start it:
```bash
MIRROR=http://apache.mirror.anlx.net/kafka/0.10.2.0/kafka_2.11-0.10.2.0.tgz

# Only download the kafka if it isn't already downloaded.
test -f kafka.tgz || wget $MIRROR -O kafka.tgz
# Unpack the kafka over the top of any existing installation
mkdir -p kafka && tar xzf kafka.tgz -C kafka --strip-components 1

# Start the zookeeper running in the background.
# By default the zookeeper listens on localhost:2181
kafka/bin/zookeeper-server-start.sh -daemon kafka/config/zookeeper.properties

# Start the kafka server running in the background.
# By default the kafka listens on localhost:9092
kafka/bin/kafka-server-start.sh -daemon kafka/config/server.properties
```

## Configuration

### Postgres database setup

Dendrite requires a postgres database engine, version 9.5 or later.

* Create role:
  ```bash
  sudo -u postgres createuser -P dendrite     # prompts for password
  ```
* Create databases:
  ```bash
  for i in account device mediaapi syncapi roomserver serverkey federationsender; do
      sudo -u postgres createdb -O dendrite dendrite_$i
  done
  ```

### Crypto key generation

Generate the keys (unlike synapse, dendrite doesn't autogen yet):

```bash
# Generate a self-signed SSL cert for federation:
test -f server.key || openssl req -x509 -newkey rsa:4096 -keyout server.key -out server.crt -days 3650 -nodes -subj /CN=localhost

# generate ed25519 signing key
test -f matrix_key.pem || python3 > matrix_key.pem <<EOF
import base64;
r = lambda n: base64.b64encode(open("/dev/urandom", "rb").read(n)).decode("utf8");
print("-----BEGIN MATRIX PRIVATE KEY-----")
print("Key-ID:", "ed25519:" + r(3).rstrip("="))
print(r(32))
print("-----END MATRIX PRIVATE KEY-----")
EOF
```

### Configuration

Create config file, based on `dendrite-config.yaml`. Call it `dendrite.yaml`. Things that will need editing include *at least*:
* `server_name`
* `database/*`


## Starting a monolith server

TODO

## Starting a multiprocess server

The following contains scripts which will run all the required processes in order to point a Matrix client at Dendrite. Conceptually, you are wiring together to form the following diagram:

```

                                                   DB:syncserver
                                                        |               roomserver_output_topic_dev
                                     /sync    +--------------------------+ <=====================
                                   +--------->| dendrite-sync-api-server |                     ||
                                   |          +--------------------------+        +----------------------+
Matrix      +------------------+   |        :7773               query API         | dendrite-room-server |--DB:roomserver
Clients --->| client-api-proxy |---+                           +----------------->+----------------------+
            +------------------+   |                           |               :7770           ^^
          :8008                    | CS API   +----------------------------+                   ||
                                   +--------->| dendrite-client-api-server |===================||
                                   |          +----------------------------+   roomserver_input_topic_dev
                                   |        :7771
                                   |
                                   | /media   +---------------------------+ 
                                   +--------->| dendrite-media-api-server |
                                              +---------------------------+
                                            :7774


   A --> B  = HTTP requests (A = client, B = server)
   A ==> B  = Kafka (A = producer, B = consumer)
```

### Run a client api proxy

This is what Matrix clients will talk to. If you use the script below, point your client at `http://localhost:8008`.

```bash
#!/bin/bash

./bin/client-api-proxy \
--bind-address ":8008" \
--sync-api-server-url "http://localhost:7773" \
--client-api-server-url "http://localhost:7771" \
--media-api-server-url "http://localhost:7774"
```

### Run a client api

This is what implements message sending. Clients talk to this via the proxy in order to send messages.

```bash
./bin/dendrite-client-api-server --config=dendrite.yaml
```

(If this fails with `pq: syntax error at or near "ON"`, check you are using at least postgres 9.5.)

### Run a room server

This is what implements the room DAG. Clients do not talk to this.

```bash
./bin/dendrite-room-server --config=dendrite.yaml
```

### Run a sync server

This is what implements `/sync` requests. Clients talk to this via the proxy in order to receive messages.

```bash
./bin/dendrite-sync-api-server --config dendrite.yaml
```

### Run a media server

This implements `/media` requests. Clients talk to this via the proxy in order to upload and retrieve media.

```bash
./bin/dendrite-media-api-server --config dendrite.yaml
```
