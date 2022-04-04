# Contributing to Dendrite

Everyone is welcome to contribute to Dendrite! We aim to make it as easy as
possible to get started.

Please ensure that you sign off your contributions! See [Sign Off](#sign-off)
section below.

## Getting up and running

See [INSTALL.md](INSTALL.md) for instructions on setting up a running dev
instance of dendrite, and [CODE_STYLE.md](CODE_STYLE.md) for the code style
guide.

We use [golangci-lint](https://github.com/golangci/golangci-lint) to lint
Dendrite which can be executed via:

```
$ golangci-lint run
```

We also have unit tests which we run via:

```
$ go test ./...
```

## Continuous Integration

When a Pull Request is submitted, continuous integration jobs are run
automatically to ensure the code builds and is relatively well-written. The jobs
are run on [Buildkite](https://buildkite.com/matrix-dot-org/dendrite/), and the
Buildkite pipeline configuration can be found in Matrix.org's [pipelines
repository](https://github.com/matrix-org/pipelines).

If a job fails, click the "details" button and you should be taken to the job's
logs.

![Click the details button on the failing build
step](https://raw.githubusercontent.com/matrix-org/dendrite/main/docs/images/details-button-location.jpg)

Scroll down to the failing step and you should see some log output. Scan the
logs until you find what it's complaining about, fix it, submit a new commit,
then rinse and repeat until CI passes.

### Running CI Tests Locally

To save waiting for CI to finish after every commit, it is ideal to run the
checks locally before pushing, fixing errors first. This also saves other people
time as only so many PRs can be tested at a given time.

To execute what Buildkite tests, first run `./build/scripts/build-test-lint.sh`; this
script will build the code, lint it, and run `go test ./...` with race condition
checking enabled. If something needs to be changed, fix it and then run the
script again until it no longer complains. Be warned that the linting can take a
significant amount of CPU and RAM.

Once the code builds, run [Sytest](https://github.com/matrix-org/sytest)
according to the guide in
[docs/sytest.md](https://github.com/matrix-org/dendrite/blob/main/docs/sytest.md#using-a-sytest-docker-image)
so you can see whether something is being broken and whether there are newly
passing tests.

If these two steps report no problems, the code should be able to pass the CI
tests.


## Picking Things To Do

If you're new then feel free to pick up an issue labelled [good first
issue](https://github.com/matrix-org/dendrite/labels/good%20first%20issue).
These should be well-contained, small pieces of work that can be picked up to
help you get familiar with the code base.

Once you're comfortable with hacking on Dendrite there are issues labelled as
[help wanted](https://github.com/matrix-org/dendrite/labels/help-wanted),
these are often slightly larger or more complicated pieces of work but are
hopefully nonetheless fairly well-contained.

We ask people who are familiar with Dendrite to leave the [good first
issue](https://github.com/matrix-org/dendrite/labels/good%20first%20issue)
issues so that there is always a way for new people to come and get involved.

## Getting Help

For questions related to developing on Dendrite we have a dedicated room on
Matrix [#dendrite-dev:matrix.org](https://matrix.to/#/#dendrite-dev:matrix.org)
where we're happy to help.

For more general questions please use
[#dendrite:matrix.org](https://matrix.to/#/#dendrite:matrix.org).

## Sign off

We ask that everyone who contributes to the project signs off their
contributions, in accordance with the
[DCO](https://github.com/matrix-org/matrix-spec/blob/main/CONTRIBUTING.rst#sign-off).
