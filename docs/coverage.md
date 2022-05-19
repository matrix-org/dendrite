---
title: Coverage
parent: Development
permalink: /development/coverage
---

To generate a test coverage report for Sytest, a small patch needs to be applied to the Sytest repository to compile and use the instrumented binary:
```patch
diff --git a/lib/SyTest/Homeserver/Dendrite.pm b/lib/SyTest/Homeserver/Dendrite.pm
index 8f0e209c..ad057e52 100644
--- a/lib/SyTest/Homeserver/Dendrite.pm
+++ b/lib/SyTest/Homeserver/Dendrite.pm
@@ -337,7 +337,7 @@ sub _start_monolith
 
    $output->diag( "Starting monolith server" );
    my @command = (
-      $self->{bindir} . '/dendrite-monolith-server',
+      $self->{bindir} . '/dendrite-monolith-server', '--test.coverprofile=' . $self->{hs_dir} . '/integrationcover.log', "DEVEL",
       '--config', $self->{paths}{config},
       '--http-bind-address', $self->{bind_host} . ':' . $self->unsecure_port,
       '--https-bind-address', $self->{bind_host} . ':' . $self->secure_port,
diff --git a/scripts/dendrite_sytest.sh b/scripts/dendrite_sytest.sh
index f009332b..7ea79869 100755
--- a/scripts/dendrite_sytest.sh
+++ b/scripts/dendrite_sytest.sh
@@ -34,7 +34,8 @@ export GOBIN=/tmp/bin
 echo >&2 "--- Building dendrite from source"
 cd /src
 mkdir -p $GOBIN
-go install -v ./cmd/dendrite-monolith-server
+# go install -v ./cmd/dendrite-monolith-server
+go test -c -cover -covermode=atomic -o $GOBIN/dendrite-monolith-server -coverpkg "github.com/matrix-org/..." ./cmd/dendrite-monolith-server
 go install -v ./cmd/generate-keys
 cd -
 ```

 Then run Sytest. This will generate a new file `integrationcover.log` in each server's directory e.g `server-0/integrationcover.log`. To parse it,
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
which will never be tested in a single test run (e.g sqlite or postgres, monolith or polylith). To get a more accurate value, additional processing is required
to remove packages which will never be tested and extension MSCs:
```bash
# These commands are all similar but change which package paths are _removed_ from the output.

# For Postgres (monolith)
go tool cover -func=/path/to/server-0/integrationcover.log | grep 'github.com/matrix-org/dendrite' | grep -Ev 'inthttp|sqlite|setup/mscs|api_trace' > coverage.txt

# For Postgres (polylith)
go tool cover -func=/path/to/server-0/integrationcover.log | grep 'github.com/matrix-org/dendrite' | grep -Ev 'sqlite|setup/mscs|api_trace' > coverage.txt

# For SQLite (monolith)
go tool cover -func=/path/to/server-0/integrationcover.log | grep 'github.com/matrix-org/dendrite' | grep -Ev 'inthttp|postgres|setup/mscs|api_trace' > coverage.txt

# For SQLite (polylith)
go tool cover -func=/path/to/server-0/integrationcover.log | grep 'github.com/matrix-org/dendrite' | grep -Ev 'postgres|setup/mscs|api_trace' > coverage.txt
```

A total value can then be calculated using:
```bash
cat coverage.txt | awk -F '\t+' '{x = x + $3} END {print x/NR}'
```


We currently do not have a way to combine Sytest/Complement/Unit Tests into a single coverage report.