# Dendrite [![Build Status](https://badge.buildkite.com/4be40938ab19f2bbc4a6c6724517353ee3ec1422e279faf374.svg?branch=master)](https://buildkite.com/matrix-dot-org/dendrite) [![Dendrite Dev on Matrix](https://img.shields.io/matrix/dendrite-dev:matrix.org.svg?label=%23dendrite-dev%3Amatrix.org&logo=matrix&server_fqdn=matrix.org)](https://matrix.to/#/#dendrite-dev:matrix.org) [![Dendrite on Matrix](https://img.shields.io/matrix/dendrite:matrix.org.svg?label=%23dendrite%3Amatrix.org&logo=matrix&server_fqdn=matrix.org)](https://matrix.to/#/#dendrite:matrix.org)

Dendrite is a second-generation Matrix homeserver written in Go. It is not recommended to use Dendrite as
a production homeserver at this time as there is no stable release. An overview of the design can be found
in [DESIGN.md](docs/DESIGN.md).

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
updates with CI. As of July 2020 we're at around 48% CS API coverage and 50% Federation coverage, though check
CI for the latest numbers. In practice, this means you can communicate locally and via federation with Synapse
servers such as matrix.org reasonably well. There's a long list of features that are not implemented, notably:
 - Receipts
 - Push
 - Search and Context
 - User Directory
 - Presence
 - Guests
 - E2E keys and device lists

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


# Contributing

Everyone is welcome to help out and contribute! See
[CONTRIBUTING.md](docs/CONTRIBUTING.md) to get started!

# Discussion

For questions about Dendrite we have a dedicated room on Matrix
[#dendrite:matrix.org](https://matrix.to/#/#dendrite:matrix.org). Development
discussion should happen in
[#dendrite-dev:matrix.org](https://matrix.to/#/#dendrite-dev:matrix.org).


