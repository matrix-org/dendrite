# Code Style

In addition to standard Go code style (`gofmt`, `goimports`), we use `golangci-lint`
to run a number of linters, the exact list can be found under linters in [.golangci.yml](.golangci.yml).
[Installation](https://github.com/golangci/golangci-lint#install) and [Editor
Integration](https://github.com/golangci/golangci-lint#editor-integration) for
it can be found in the readme of golangci-lint.

For rare cases where a linter is giving a spurious warning, it can be disabled
for that line or statement using a [comment
directive](https://github.com/golangci/golangci-lint#nolint), e.g.  `var
bad_name int //nolint:golint,unused`. This should be used sparingly and only
when its clear that the lint warning is spurious.

The linters can be run using [build/scripts/find-lint.sh](/build/scripts/find-lint.sh)
(see file for docs) or as part of a build/test/lint cycle using
[build/scripts/build-test-lint.sh](/build/scripts/build-test-lint.sh).


## Labels

In addition to `TODO` and `FIXME` we also use `NOTSPEC` to identify deviations
from the Matrix specification.

## Logging

We generally prefer to log with static log messages and include any dynamic
information in fields.

```golang
logger := util.GetLogger(ctx)

// Not recommended
logger.Infof("Finished processing keys for %s, number of keys %d", name, numKeys)

// Recommended
logger.WithFields(logrus.Fields{
    "numberOfKeys": numKeys,
    "entityName":   name,
}).Info("Finished processing keys")
```

This is useful when logging to systems that natively understand log fields, as
it allows people to search and process the fields without having to parse the
log message.


## Visual Studio Code

If you use VSCode then the following is an example of a workspace setting that
sets up linting correctly:

```json
{
    "go.lintTool":"golangci-lint",
    "go.lintFlags": [
      "--fast"
    ]
}
```
