# Dendrite
[![Build status](https://github.com/matrix-org/dendrite/actions/workflows/dendrite.yml/badge.svg?event=push)](https://github.com/matrix-org/dendrite/actions/workflows/dendrite.yml) [![Dendrite](https://img.shields.io/matrix/dendrite:matrix.org.svg?label=%23dendrite%3Amatrix.org&logo=matrix&server_fqdn=matrix.org)](https://matrix.to/#/#dendrite:matrix.org) [![Dendrite Dev](https://img.shields.io/matrix/dendrite-dev:matrix.org.svg?label=%23dendrite-dev%3Amatrix.org&logo=matrix&server_fqdn=matrix.org)](https://matrix.to/#/#dendrite-dev:matrix.org)

Dendrite is a second-generation Matrix homeserver written in Go.
It intends to provide an **efficient**, **reliable** and **scalable** alternative to [Synapse](https://github.com/matrix-org/synapse):

- Efficient: A small memory footprint with better baseline performance than an out-of-the-box Synapse.
- Reliable: Implements the Matrix specification as written, using the
   [same test suite](https://github.com/matrix-org/sytest) as Synapse as well as
   a [brand new Go test suite](https://github.com/matrix-org/complement).
- Scalable: can run on multiple machines and eventually scale to massive homeserver deployments.

As of October 2020, Dendrite has now entered **beta** which means:

- Dendrite is ready for early adopters. We recommend running in Monolith mode with a PostgreSQL database.
- Dendrite has periodic semver releases. We intend to release new versions as we land significant features.
- Dendrite supports database schema upgrades between releases. This means you should never lose your messages when upgrading Dendrite.
- Breaking changes will not occur on minor releases. This means you can safely upgrade Dendrite without modifying your database or config file.

This does not mean:

- Dendrite is bug-free. It has not yet been battle-tested in the real world and so will be error prone initially.
- All of the CS/Federation APIs are implemented. We are tracking progress via a script called 'Are We Synapse Yet?'. In particular,
   presence and push notifications are entirely missing from Dendrite. See [CHANGES.md](CHANGES.md) for updates.
- Dendrite is ready for massive homeserver deployments. You cannot shard each microservice, only run each one on a different machine.

Currently, we expect Dendrite to function well for small (10s/100s of users) homeserver deployments as well as P2P Matrix nodes in-browser or on mobile devices.
In the future, we will be able to scale up to gigantic servers (equivalent to matrix.org) via polylith mode.

If you have further questions, please take a look at [our FAQ](docs/FAQ.md) or join us in:

- **[#dendrite:matrix.org](https://matrix.to/#/#dendrite:matrix.org)** - General chat about the Dendrite project, for users and server admins alike
- **[#dendrite-dev:matrix.org](https://matrix.to/#/#dendrite-dev:matrix.org)** - The place for developers, where all Dendrite development discussion happens
- **[#dendrite-alerts:matrix.org](https://matrix.to/#/#dendrite-alerts:matrix.org)** - Release notifications and important info, highly recommended for all Dendrite server admins

## Requirements

To build Dendrite, you will need Go 1.16 or later.

For a usable federating Dendrite deployment, you will also need:

- A domain name (or subdomain)
- A valid TLS certificate issued by a trusted authority for that domain
- SRV records or a well-known file pointing to your deployment

Also recommended are:

- A PostgreSQL database engine, which will perform better than SQLite with many users and/or larger rooms
- A reverse proxy server, such as nginx, configured [like this sample](https://github.com/matrix-org/dendrite/blob/master/docs/nginx/monolith-sample.conf)

The [Federation Tester](https://federationtester.matrix.org) can be used to verify your deployment.

## Get started

If you wish to build a fully-federating Dendrite instance, see [INSTALL.md](docs/INSTALL.md). For running in Docker, see [build/docker](build/docker).

The following instructions are enough to get Dendrite started as a non-federating test deployment using self-signed certificates and SQLite databases:

```bash
$ git clone https://github.com/matrix-org/dendrite
$ cd dendrite
$ ./build.sh

# Generate a Matrix signing key for federation (required)
$ ./bin/generate-keys --private-key matrix_key.pem

# Generate a self-signed certificate (optional, but a valid TLS certificate is normally
# needed for Matrix federation/clients to work properly!)
$ ./bin/generate-keys --tls-cert server.crt --tls-key server.key

# Copy and modify the config file - you'll need to set a server name and paths to the keys
# at the very least, along with setting up the database connection strings.
$ cp dendrite-config.yaml dendrite.yaml

# Build and run the server:
$ ./bin/dendrite-monolith-server --tls-cert server.crt --tls-key server.key --config dendrite.yaml
```

Then point your favourite Matrix client at `http://localhost:8008` or `https://localhost:8448`.

## Progress

We use a script called Are We Synapse Yet which checks Sytest compliance rates. Sytest is a black-box homeserver
test rig with around 900 tests. The script works out how many of these tests are passing on Dendrite and it
updates with CI. As of April 2022 we're at around 83% CS API coverage and 95% Federation coverage, though check
CI for the latest numbers. In practice, this means you can communicate locally and via federation with Synapse
servers such as matrix.org reasonably well, although there are still some missing features (like Search).

We are prioritising features that will benefit single-user homeservers first (e.g Receipts, E2E) rather
than features that massive deployments may be interested in (User Directory, OpenID, Guests, Admin APIs, AS API).
This means Dendrite supports amongst others:

- Core room functionality (creating rooms, invites, auth rules)
- Full support for room versions 1 to 7
- Experimental support for room versions 8 to 9
- Backfilling locally and via federation
- Accounts, Profiles and Devices
- Published room lists
- Typing
- Media APIs
- Redaction
- Tagging
- Context
- E2E keys and device lists
- Receipts
- Push
- Guests
- User Directory
- Presence

## Contributing

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

## Hardware requirements

Dendrite in Monolith + SQLite works in a range of environments including iOS and in-browser via WASM.

For small homeserver installations joined on ~10s rooms on matrix.org with ~100s of users in those rooms, including some
encrypted rooms:

- Memory: uses around 100MB of RAM, with peaks at around 200MB.
- Disk space: After a few months of usage, the database grew to around 2GB (in Monolith mode).
- CPU: Brief spikes when processing events, typically idles at 1% CPU.

This means Dendrite should comfortably work on things like Raspberry Pis.
