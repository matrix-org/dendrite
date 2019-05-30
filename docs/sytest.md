# SyTest

Dendrite uses [SyTest](https://github.com/matrix-org/sytest) for its
integration testing. When creating a new PR, add the test IDs (see below) that
your PR should allow to pass to `testfile` in dendrite's root directory. Not all
PRs need to make new tests pass. If we find your PR should be making a test pass
we may ask you to add to that file, as generally Dendrite's progress can be
tracked through the amount of SyTest tests it passes.

## Finding out which tests to add

An easy way to do this will be let CircleCI run sytest, and then search for the
tests that should be added in the test results file.

You may enable [CircleCI](https://circleci.com/) for your own fork, or simply
refer to the CircleCI build for your PR. Once the build is complete, you can
find the test results in `Container N/logs/results.tap` under the **Artifacts**
tab on the CircleCI build page. To get the tests that should be added, search
for the string:

```
# TODO passed but expected fail
```

Generally, tests with this label should be the tests that your PR is allowing to
pass.

To add a test into `testfile`, extract its test ID and append it to `testfile`.
For example, to add this test:

```
ok 4 (expected fail) POST /register rejects registration of usernames with '!' # TODO passed but expected fail
```

Append:

```
POST /register rejects registration of usernames with '!'
```

to `testfile`.
