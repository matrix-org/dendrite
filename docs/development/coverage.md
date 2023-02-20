---
title: Coverage
parent: Development
nav_order: 3
permalink: /development/coverage
---

## Running Sytest with coverage enabled

To run Sytest with coverage enabled:

```bash 
docker run --rm --name sytest -v "/Users/kegan/github/sytest:/sytest" \
-v "/Users/kegan/github/dendrite:/src" -v "/Users/kegan/logs:/logs" \
-v "/Users/kegan/go/:/gopath" -e "POSTGRES=1" -e "DENDRITE_TRACE_HTTP=1" \
-e "COVER=1" \
matrixdotorg/sytest-dendrite:latest
```

This will generate a new file `integrationcover.log` in each server's directory, e.g `server-0/integrationcover.log`. To parse it,
 ensure your working directory is under the Dendrite repository then run:

 ```bash
 go tool cover -func=/path/to/server-0/integrationcover.log
 ```
 which will produce an output like:
 ```
 ...
 github.com/matrix-org/util/json.go:83:											NewJSONRequestHandler				100.0%
github.com/matrix-org/util/json.go:90:											Protect						57.1%
github.com/matrix-org/util/json.go:110:											RequestWithLogging				100.0%
github.com/matrix-org/util/json.go:132:											MakeJSONAPI					70.0%
github.com/matrix-org/util/json.go:151:											respond						61.5%
github.com/matrix-org/util/json.go:180:											WithCORSOptions					0.0%
github.com/matrix-org/util/json.go:191:											SetCORSHeaders					100.0%
github.com/matrix-org/util/json.go:202:											RandomString					100.0%
github.com/matrix-org/util/json.go:210:											init						100.0%
github.com/matrix-org/util/unique.go:13:										Unique						91.7%
github.com/matrix-org/util/unique.go:48:										SortAndUnique					100.0%
github.com/matrix-org/util/unique.go:55:										UniqueStrings					100.0%
total:															(statements)					53.7%
```

The total coverage for this run is the last line at the bottom. However, this value is misleading because Dendrite can run in many different configurations,
which will never be tested in a single test run (e.g sqlite or postgres). To get a more accurate value, additional processing is required
to remove packages which will never be tested and extension MSCs:

(The following commands use [gocovmerge](https://github.com/wadey/gocovmerge), to merge two or more coverage logs, so make sure you have that installed)

```bash
# These commands are all similar but change which package paths are _removed_ from the output.

# For Postgres
find -name 'integrationcover.log' | xargs gocovmerge | grep -Ev 'relayapi|sqlite|setup/mscs' > final.cov && go tool cover -func=final.cov

# For SQLite
find -name 'integrationcover.log' | xargs gocovmerge | grep -Ev 'relayapi|postgres|setup/mscs' > final.cov && go tool cover -func=final.cov
```

## Running unit tests with coverage enabled

Running unit tests with coverage enabled can be done with the following command, it will also generate a `integrationcover.log`, which can later be merged
using `gocovmerge`:
```bash
go test -covermode=atomic -coverpkg=./... -coverprofile=integrationcover.log $(go list ./... | grep -v '/cmd/')
```

## Getting coverage from Complement

Getting the coverage for Complement runs is a bit more involved.

First you'll need a docker image compatible with Complement, one can be built using
```bash
docker build -t complement-dendrite -f build/scripts/Complement.Dockerfile .
```
from within the Dendrite repository.

Clone complement to a directory of your liking:
```bash
git clone https://github.com/matrix-org/complement.git
cd complement
```

Next we'll need a script to execute after a test finishes, create a new file `posttest.sh`, make the file executable (`chmod +x posttest.sh`) 
and add the following content:
```bash
#!/bin/bash

mkdir -p /tmp/Complement/logs/$2/$1/
docker cp $1:/dendrite/integrationcover.log /tmp/Complement/logs/$2/$1/
```
This will copy the `integrationcover.log` files from each container to something like 
`/tmp/Complement/logs/TestLogin/94f9c428de95779d2b62a3ccd8eab9d5ddcf65cc259a40ece06bdc61687ffed3/integrationcover.log`. (`$1` is the containerID, `$2` the test name)

Now that we have set up everything we need, we can finally execute Complement:
```bash
COMPLEMENT_BASE_IMAGE=complement-dendrite \
COMPLEMENT_SHARE_ENV_PREFIX=COMPLEMENT_DENDRITE_ \
COMPLEMENT_DENDRITE_COVER=1 \
COMPLEMENT_POST_TEST_SCRIPT=$(pwd)/posttest.sh \
  go test -tags dendrite_blacklist ./tests/... -count=1 -v -timeout=30m -failfast=false
```

Once this is done, you can copy the resulting `integrationcover.log` files to your Dendrite repository for the next step.
```bash
cp -pr /tmp/Complement/logs PathToYourDendriteRepository
```

## Combining the results of all runs

Now that we have all our `integrationcover.log` files within the Dendrite repository, you can now execute the following command, to get the coverage
overall (Note: update `grep -Ev` accordingly, to exclude SQLite for example):
```bash
find -name 'integrationcover.log' | xargs gocovmerge | grep -Ev 'setup/mscs|api_trace' > final.cov && go tool cover -func=final.cov
```