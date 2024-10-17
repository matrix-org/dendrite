---
title: SyTest
parent: Development
nav_order: 2
permalink: /development/sytest
---

# SyTest

Dendrite uses [SyTest](https://github.com/matrix-org/sytest) for its
integration testing. When creating a new PR, add the test IDs (see below) that
your PR should allow to pass to `sytest-whitelist` in dendrite's root
directory. Not all PRs need to make new tests pass. If we find your PR should
be making a test pass we may ask you to add to that file, as generally
Dendrite's progress can be tracked through the amount of SyTest tests it
passes.

## Finding out which tests to add

We recommend you run the tests locally by using the SyTest docker image.
After running the tests, a script will print the tests you need to add to
`sytest-whitelist`.

You should proceed after you see no build problems for dendrite after running:

```sh
go build -o bin/ ./cmd/...
```

If you are fixing an issue marked with
[Are We Synapse Yet](https://github.com/element-hq/dendrite/labels/are-we-synapse-yet)
then there will be a list of Sytests that you should add to the whitelist when you
have fixed that issue. This MUST be included in your PR to ensure that the issue
is fully resolved.

### Using the SyTest Docker image

**We strongly recommend using the Docker image to run Sytest.**

Use the following commands to pull the latest SyTest image and run the tests:

```sh
docker pull matrixdotorg/sytest-dendrite
docker run --rm -v /path/to/dendrite/:/src/ -v /path/to/log/output/:/logs/ matrixdotorg/sytest-dendrite
```

`/path/to/dendrite/` should be replaced with the actual path to your dendrite
source code. The test results TAP file and homeserver logging output will go to
`/path/to/log/output`. The output of the command should tell you if you need to
add any tests to `sytest-whitelist`.

When debugging, the following Docker `run` options may also be useful:

* `-v /path/to/sytest/:/sytest/`: Use your local SyTest repository at
  `/path/to/sytest` instead of pulling from GitHub. This is useful when you want
  to speed things up or make modifications to SyTest.
* `-v "/path/to/gopath/:/gopath"`: Use your local `GOPATH` so you don't need to
  re-download packages on every run.
* `--entrypoint bash`: Prevent the container from automatically starting the
  tests.  When used, you need to manually run `/bootstrap.sh dendrite` inside
  the container to start them.
* `-e "DENDRITE_TRACE_HTTP=1"`: Adds HTTP tracing to server logs.
* `-e "DENDRITE_TRACE_INTERNAL=1"`: Adds roomserver internal API tracing to
  server logs.
* `-e "COVER=1"`: Run Sytest with an instrumented binary, producing a Go coverage file per server.
* `-e "RACE_DETECTION=1"`: Build the binaries with the `-race` flag (Note: This will significantly slow down test runs)

The docker command also supports a single positional argument for the test file to
run, so you can run a single `.pl` file rather than the whole test suite. For example:

```
docker run --rm --name sytest -v "/Users/kegan/github/sytest:/sytest"
-v "/Users/kegan/github/dendrite:/src" -v "/Users/kegan/logs:/logs"
-v "/Users/kegan/go/:/gopath" -e "POSTGRES=1" -e "DENDRITE_TRACE_HTTP=1"
matrixdotorg/sytest-dendrite:latest tests/50federation/40devicelists.pl
```
