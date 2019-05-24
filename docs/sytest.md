# SyTest

Dendrite uses [SyTest](https://github.com/matrix-org/sytest) for its
integration testing. When creating a new PR, add the test IDs that your PR
should allow to pass to `testfile` in dendrite's root directory. Not all PRs
need to make new tests pass. If we find your PR should be making a test pass we
may ask you to add to that file, as generally Dendrite's progress can be
tracked through the amount of SyTest tests it passes.
