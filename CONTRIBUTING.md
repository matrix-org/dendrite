# Contributing to Dendrite

Everyone is welcome to contribute to Dendrite! We aim to make it as easy as
possible to get started.

Please ensure that you sign off your contributions! See [Sign Off](#sign-off)
section below.

## Getting up and running

See [INSTALL.md](INSTALL.md) for instructions on setting up a running dev
instance of dendrite, and [CODE_STYLE.md](CODE_STYLE.md) for the code style
guide.

We use `gb` for managing our dependencies, so `gb build` and `gb test` is how
to build dendrite and run the unit tests respectively. Be aware that a list of
all dendrite packages is the expected output for all tests succeeding with `gb
test`. There are also [scripts](scripts) for [linting](scripts/find-lint.sh)
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

## Contributing to dependencies

Dependencies are located in `vendor/src` and are managed by `gb`. If you need
to make some changes in those directories, you first need to open a PR in the
dependency repository. Once your PR is merged, you need to run `gb vendor
update $repo_url` (example: `gb vendor update github.com/matrix-org/gomatrix`)
in the dendrite repository to update the dependency.

You can then create a commit containing only the modified vendor files (along
with the `vendor/manifest` file), name it with the command you just ran (ie
`gb vendor update github.com/matrix-org/gomatrix`), and open a PR on Dendrite.

## Getting Help

For questions related to developing on Dendrite we have a dedicated room on
Matrix [#dendrite-dev:matrix.org](https://matrix.to/#/#dendrite-dev:matrix.org)
where we're happy to help.

For more general questions please use [#dendrite:matrix.org](https://matrix.to/#/#dendrite:matrix.org).

## Sign off

We ask that everyone who contributes to the project signs off their
contributions, in accordance with the [DCO](https://github.com/matrix-org/matrix-doc/blob/master/CONTRIBUTING.rst#sign-off).

