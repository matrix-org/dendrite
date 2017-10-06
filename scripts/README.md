# Dev Scripts

These are a collection of scripts that should be helpful for those developing
on dendrite.

- `./scripts/find-lint.sh` runs the linters against dendrite,
  `./scripts/find-lint.sh fast` runs a subset of faster lints
- `./scripts/build-test-lint.sh` builds, tests and lints dendrite, and
  should be run before pushing commits
- `./scripts/install-local-kafka.sh` downloads, installs and runs a
  kafka instance
- `./scripts/travis-test.sh` is what travis runs


The linters can take a lot of resources and are slow, so they can be
configured using two enviroment variables:

- `DENDRITE_LINT_CONCURRENCY` - number of concurrent linters to run,
  gometalinter defaults this to 8
- `DENDRITE_LINT_DISABLE_GC` - if set then the the go gc will be disabled
  when running the linters, speeding them up but using much more memory.
