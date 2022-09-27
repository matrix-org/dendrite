---
title: Contributing
parent: Development
permalink: /development/contributing
---

# Contributing to Dendrite

Everyone is welcome to contribute to Dendrite! We aim to make it as easy as
possible to get started.

## Sign off

We require that everyone who contributes to the project signs off their contributions
in accordance with the [Developer Certificate of Origin](https://github.com/matrix-org/matrix-spec/blob/main/CONTRIBUTING.rst#sign-off).
In effect, this means adding a statement to your pull requests or commit messages
along the lines of:

```
Signed-off-by: Full Name <email address>
```

Unfortunately we can't accept contributions without a sign-off.

Please note that we can only accept contributions under a legally identifiable name,
such as your name as it appears on government-issued documentation or common-law names
(claimed by legitimate usage or repute). We cannot accept sign-offs from a pseudonym or
alias and cannot accept anonymous contributions.

If you would prefer to sign off privately instead (so as to not reveal your full
name on a public pull request), you can do so by emailing a sign-off declaration
and a link to your pull request directly to the [Matrix.org Foundation](https://matrix.org/foundation/)
at `dco@matrix.org`. Once a private sign-off has been made, you will not be required
to do so for future contributions.

## Getting up and running

See the [Installation](installation) section for information on how to build an
instance of Dendrite. You will likely need this in order to test your changes.

## Code style

On the whole, the format as prescribed by `gofmt`, `goimports` etc. is exactly
what we use and expect. Please make sure that you run one of these formatters before
submitting your contribution.

## Comments

Please make sure that the comments adequately explain *why* your code does what it
does. If there are statements that are not obvious, please comment what they do.

We also have some special tags which we use for searchability. These are:

* `// TODO:` for places where a future review, rewrite or refactor is likely required;
* `// FIXME:` for places where we know there is an outstanding bug that needs a fix;
* `// NOTSPEC:` for places where the behaviour specifically does not match what the
  [Matrix Specification](https://spec.matrix.org/) prescribes, along with a description
  of *why* that is the case.

## Linting

We use [golangci-lint](https://github.com/golangci/golangci-lint) to lint Dendrite
which can be executed via:

```bash
golangci-lint run
```

If you are receiving linter warnings that you are certain are spurious and want to
silence them, you can annotate the relevant lines or methods with a `// nolint:`
comment. Please avoid doing this if you can.

## Unit tests

We also have unit tests which we run via:

```bash
go test --race ./...
```

In general, we like submissions that come with tests. Anything that proves that the
code is functioning as intended is great, and to ensure that we will find out quickly
in the future if any regressions happen.

We use the standard [Go testing package](https://gobyexample.com/testing) for this,
alongside some helper functions in our own [`test` package](https://pkg.go.dev/github.com/matrix-org/dendrite/test).

## Continuous integration

When a Pull Request is submitted, continuous integration jobs are run automatically
by GitHub actions to ensure that the code builds and works in a number of configurations,
such as different Go versions, using full HTTP APIs and both database engines.
CI will automatically run the unit tests (as above) as well as both of our integration
test suites ([Complement](https://github.com/matrix-org/complement) and
[SyTest](https://github.com/matrix-org/sytest)).

You can see the progress of any CI jobs at the bottom of the Pull Request page, or by
looking at the [Actions](https://github.com/matrix-org/dendrite/actions) tab of the Dendrite
repository.

We generally won't accept a submission unless all of the CI jobs are passing. We
do understand though that sometimes the tests get things wrong — if that's the case,
please also raise a pull request to fix the relevant tests!

### Running CI tests locally

To save waiting for CI to finish after every commit, it is ideal to run the
checks locally before pushing, fixing errors first. This also saves other people
time as only so many PRs can be tested at a given time.

To execute what CI tests, first run `./build/scripts/build-test-lint.sh`; this
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

## Picking things to do

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

## Getting help

For questions related to developing on Dendrite we have a dedicated room on
Matrix [#dendrite-dev:matrix.org](https://matrix.to/#/#dendrite-dev:matrix.org)
where we're happy to help.

For more general questions please use [#dendrite:matrix.org](https://matrix.to/#/#dendrite:matrix.org).
