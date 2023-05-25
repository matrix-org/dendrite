---
title: Coverage
parent: Development
nav_order: 3
permalink: /development/coverage
---

## Running unit tests with coverage enabled

Running unit tests with coverage enabled can be done with the following commands, this will generate a `integrationcover.log`
```bash
go test -covermode=atomic -coverpkg=./... -coverprofile=integrationcover.log $(go list ./... | grep -v '/cmd/')
go tool cover -func=integrationcover.log
```

## Running Sytest with coverage enabled

To run Sytest with coverage enabled:

```bash 
docker run --rm --name sytest -v "/Users/kegan/github/sytest:/sytest" \
  -v "/Users/kegan/github/dendrite:/src" -v "$(pwd)/sytest_logs:/logs" \
  -v "/Users/kegan/go/:/gopath" -e "POSTGRES=1" \
  -e "COVER=1" \
  matrixdotorg/sytest-dendrite:latest
  
# to get a more accurate coverage you may also need to run Sytest using SQLite as the database:
docker run --rm --name sytest -v "/Users/kegan/github/sytest:/sytest" \
  -v "/Users/kegan/github/dendrite:/src" -v "$(pwd)/sytest_logs:/logs" \
  -v "/Users/kegan/go/:/gopath" \
  -e "COVER=1" \
  matrixdotorg/sytest-dendrite:latest
```

This will generate a folder `covdatafiles` in each server's directory, e.g `server-0/covdatafiles`. To parse them,
ensure your working directory is under the Dendrite repository then run:

 ```bash
 go tool covdata func -i="$(find -name 'covmeta*' -type f -exec dirname {} \; | uniq | paste -s -d ',' -)"
 ```
 which will produce an output like:
 ```
 ...
github.com/matrix-org/util/json.go:132:   MakeJSONAPI         70.0%
github.com/matrix-org/util/json.go:151:   respond             84.6%
github.com/matrix-org/util/json.go:180:   WithCORSOptions      0.0%
github.com/matrix-org/util/json.go:191:   SetCORSHeaders     100.0%
github.com/matrix-org/util/json.go:202:   RandomString       100.0%
github.com/matrix-org/util/json.go:210:   init               100.0%
github.com/matrix-org/util/unique.go:13:  Unique              91.7%
github.com/matrix-org/util/unique.go:48:  SortAndUnique      100.0%
github.com/matrix-org/util/unique.go:55:  UniqueStrings      100.0%
total                                    (statements)         64.0%
```
(after running Sytest for Postgres _and_ SQLite)

The total coverage for this run is the last line at the bottom. However, this value is misleading because Dendrite can run in different configurations,
which will never be tested in a single test run (e.g sqlite or postgres). To get a more accurate value, you'll need run Sytest for Postgres and SQLite (see commands above).
Additional processing is required also to remove packages which will never be tested and extension MSCs:

```bash
# If you executed both commands from above, you can get the total coverage using the following commands  
go tool covdata textfmt -i="$(find -name 'covmeta*' -type f -exec dirname {} \; | uniq | paste -s -d ',' -)" -o sytest.cov
grep -Ev 'relayapi|setup/mscs' sytest.cov > final.cov
go tool cover -func=final.cov

# If you only executed the one for Postgres:
go tool covdata textfmt -i="$(find -name 'covmeta*' -type f -exec dirname {} \; | uniq | paste -s -d ',' -)" -o sytest.cov
grep -Ev 'relayapi|sqlite|setup/mscs' sytest.cov > final.cov
go tool cover -func=final.cov

# If you only executed the one for SQLite:
go tool covdata textfmt -i="$(find -name 'covmeta*' -type f -exec dirname {} \; | uniq | paste -s -d ',' -)" -o sytest.cov
grep -Ev 'relayapi|postgres|setup/mscs' sytest.cov > final.cov
go tool cover -func=final.cov
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
docker cp $1:/tmp/covdatafiles/. /tmp/Complement/logs/$2/$1/
```
This will copy the `covdatafiles` files from each container to something like 
`/tmp/Complement/logs/TestLogin/94f9c428de95779d2b62a3ccd8eab9d5ddcf65cc259a40ece06bdc61687ffed3/`. (`$1` is the containerID, `$2` the test name)

Now that we have set up everything we need, we can finally execute Complement:
```bash
COMPLEMENT_BASE_IMAGE=complement-dendrite \
COMPLEMENT_SHARE_ENV_PREFIX=COMPLEMENT_DENDRITE_ \
COMPLEMENT_DENDRITE_COVER=1 \
COMPLEMENT_POST_TEST_SCRIPT=$(pwd)/posttest.sh \
  go test -tags dendrite_blacklist ./tests/... -count=1 -v -timeout=30m -failfast=false
```

Once this is done, you can copy the resulting `covdatafiles` files to your Dendrite repository for the next step.
```bash
cp -pr /tmp/Complement/logs PathToYourDendriteRepository
```

You can also run the following to get the coverage for Complement runs alone:
```bash
go tool covdata func -i="$(find /tmp/Complement -name 'covmeta*' -type f -exec dirname {} \; | uniq | paste -s -d ',' -)"
```

## Combining the results of (almost) all runs

Now that we have all our `covdatafiles` files within the Dendrite repository, you can now execute the following command, to get the coverage
overall (excluding unit tests):
```bash
go tool covdata func -i="$(find -name 'covmeta*' -type f -exec dirname {} \; | uniq | paste -s -d ',' -)"
```