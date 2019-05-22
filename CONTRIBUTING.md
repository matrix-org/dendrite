# Contributing to Dendrite

Everyone is welcome to contribute to Dendrite! We aim to make it as easy as
possible to get started.

Please ensure that you sign off your contributions! See [Sign Off](#sign-off)
section below.

## Getting up and running

See [INSTALL.md](INSTALL.md) for instructions on setting up a running dev
instance of dendrite, and [CODE_STYLE.md](CODE_STYLE.md) for the code style
guide.

As of May 2019, we're not using `gb` anymore, which is the tool we had been
using for managing our dependencies, and switched to Go modules. To build
Dendrite, run the `build.sh` script at the root of this repository, and to run
unit tests, run `go test ./...` (which should pick up any unit test and run
them). There are also [scripts](scripts) for [linting](scripts/find-lint.sh)
and doing a [build/test/lint run](scripts/build-test-lint.sh).


## Picking Things To Do

If you're new then feel free to pick up an issue labelled [good first issue](https://github.com/matrix-org/dendrite/labels/good%20first%20issue).
These should be well-contained, small pieces of work that can be picked up to
help you get familiar with the code base.

Once you're comfortable with hacking on Dendrite there are issues lablled as
[help wanted](https://github.com/matrix-org/dendrite/labels/help%20wanted), these
are often slightly larger or more complicated pieces of work but are hopefully
nonetheless fairly well-contained.

We ask people who are familiar with Dendrite to leave the [good first issue](https://github.com/matrix-org/dendrite/labels/good%20first%20issue)
issues so that there is always a way for new people to come and get involved.

## Getting Help

For questions related to developing on Dendrite we have a dedicated room on
Matrix [#dendrite-dev:matrix.org](https://matrix.to/#/#dendrite-dev:matrix.org)
where we're happy to help.

For more general questions please use [#dendrite:matrix.org](https://matrix.to/#/#dendrite:matrix.org).

## Sign off

We ask that everyone who contributes to the project signs off their
contributions, in accordance with the [DCO](https://github.com/matrix-org/matrix-doc/blob/master/CONTRIBUTING.rst#sign-off).
