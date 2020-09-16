# Dendrite [![Build Status](https://badge.buildkite.com/4be40938ab19f2bbc4a6c6724517353ee3ec1422e279faf374.svg?branch=master)](https://buildkite.com/matrix-dot-org/dendrite) [![Dendrite Dev on Matrix](https://img.shields.io/matrix/dendrite-dev:matrix.org.svg?label=%23dendrite-dev%3Amatrix.org&logo=matrix&server_fqdn=matrix.org)](https://matrix.to/#/#dendrite-dev:matrix.org) [![Dendrite on Matrix](https://img.shields.io/matrix/dendrite:matrix.org.svg?label=%23dendrite%3Amatrix.org&logo=matrix&server_fqdn=matrix.org)](https://matrix.to/#/#dendrite:matrix.org)

Dendrite is a second-generation Matrix homeserver written in Go. It is not recommended to use Dendrite as
a production homeserver at this time as there is no stable release.

Dendrite will start to receive versioned releases stable enough to run [once we enter beta](https://github.com/matrix-org/dendrite/milestone/8).

# Quick start

Requires Go 1.13+ and SQLite3 (Postgres is also supported):

```bash
$ git clone https://github.com/matrix-org/dendrite
$ cd dendrite

# generate self-signed certificate and an event signing key for federation
$ go build ./cmd/generate-keys
$ ./generate-keys --private-key matrix_key.pem --tls-cert server.crt --tls-key server.key

# Copy and modify the config file:
# you'll need to set a server name and paths to the keys at the very least, along with setting
# up the database filenames
$ cp dendrite-config.yaml dendrite.yaml

# build and run the server
$ go build ./cmd/dendrite-monolith-server
$ ./dendrite-monolith-server --tls-cert server.crt --tls-key server.key --config dendrite.yaml
```

Then point your favourite Matrix client at `http://localhost:8008`. For full installation information, see
[INSTALL.md](docs/INSTALL.md). For running in Docker, see [build/docker](build/docker).

# Progress

We use a script called Are We Synapse Yet which checks Sytest compliance rates. Sytest is a black-box homeserver
test rig with around 900 tests. The script works out how many of these tests are passing on Dendrite and it
updates with CI. As of August 2020 we're at around 52% CS API coverage and 65% Federation coverage, though check
CI for the latest numbers. In practice, this means you can communicate locally and via federation with Synapse
servers such as matrix.org reasonably well. There's a long list of features that are not implemented, notably:
 - Receipts
 - Push
 - Search and Context
 - User Directory
 - Presence
 - Guests

We are prioritising features that will benefit single-user homeservers first (e.g Receipts, E2E) rather
than features that massive deployments may be interested in (User Directory, OpenID, Guests, Admin APIs, AS API).
This means Dendrite supports amongst others:
 - Core room functionality (creating rooms, invites, auth rules)
 - Federation in rooms v1-v6
 - Backfilling locally and via federation
 - Accounts, Profiles and Devices
 - Published room lists
 - Typing
 - Media APIs
 - Redaction
 - Tagging
 - E2E keys and device lists


# Contributing

We would be grateful for any help on issues marked as
[Are We Synapse Yet](https://github.com/matrix-org/dendrite/labels/are-we-synapse-yet). These issues
all have related Sytests which need to pass in order for the issue to be closed. Once you've written your
code, you can quickly run Sytest to ensure that the test names are now passing.

For example, if the test `Local device key changes get to remote servers` was marked as failing, find the
test file (e.g via `grep` or via the
[CI log output](https://buildkite.com/matrix-dot-org/dendrite/builds/2826#39cff5de-e032-4ad0-ad26-f819e6919c42)
it's `tests/50federation/40devicelists.pl` ) then to run Sytest:
```
docker run --rm --name sytest
-v "/Users/kegan/github/sytest:/sytest"
-v "/Users/kegan/github/dendrite:/src"
-v "/Users/kegan/logs:/logs"
-v "/Users/kegan/go/:/gopath"
-e "POSTGRES=1" -e "DENDRITE_TRACE_HTTP=1"
matrixdotorg/sytest-dendrite:latest tests/50federation/40devicelists.pl
```
See [sytest.md](docs/sytest.md) for the full description of these flags.

You can try running sytest outside of docker for faster runs, but the dependencies can be temperamental
and we recommend using docker where possible.
```
cd sytest
export PERL5LIB=$HOME/lib/perl5
export PERL_MB_OPT=--install_base=$HOME
export PERL_MM_OPT=INSTALL_BASE=$HOME
./install-deps.pl

./run-tests.pl -I Dendrite::Monolith -d $PATH_TO_DENDRITE_BINARIES
```

Sometimes Sytest is testing the wrong thing or is flakey, so it will need to be patched.
Ask on `#dendrite-dev:matrix.org` if you think this is the case for you and we'll be happy to help.

If you're new to the project, see [CONTRIBUTING.md](docs/CONTRIBUTING.md) to get up to speed then
look for [Good First Issues](https://github.com/matrix-org/dendrite/labels/good%20first%20issue). If you're
familiar with the project, look for [Help Wanted](https://github.com/matrix-org/dendrite/labels/help-wanted)
issues.

# Hardware requirements

Dendrite in Monolith + SQLite works in a range of environments including iOS and in-browser via WASM.

For small homeserver installations joined on ~10s rooms on matrix.org with ~100s of users in those rooms, including some
encrypted rooms:
 - Memory: uses around 100MB of RAM, with peaks at around 200MB.
 - Disk space: After a few months of usage, the database grew to around 2GB (in Monolith mode).
 - CPU: Brief spikes when processing events, typically idles at 1% CPU.

This means Dendrite should comfortably work on things like Raspberry Pis.

# Discussion

For questions about Dendrite we have a dedicated room on Matrix
[#dendrite:matrix.org](https://matrix.to/#/#dendrite:matrix.org). Development
discussion should happen in
[#dendrite-dev:matrix.org](https://matrix.to/#/#dendrite-dev:matrix.org).


