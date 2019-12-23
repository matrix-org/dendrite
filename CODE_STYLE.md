# Code Style

We follow the standard Go style using gofmt, but with a few extra
considerations.

## Linters

We use `golangci-lint` to run a number of linters, the exact list can be found under linters
in [.golangci.yml](.golangci.yml). [Installation](https://github.com/golangci/golangci-lint#install) and [Editor Integration](https://github.com/golangci/golangci-lint#editor-integration) for it can be found in the readme of golangci-lint.

For rare cases where a linter is giving a spurious warning, it can be disabled
for that line or statement using a [comment directive](https://github.com/alecthomas/gometalinter#comment-directives), e.g.
`// nolint: gocyclo`. This should be used sparingly and only when its clear
that the lint warning is spurious.

The linters can be run using [scripts/find-lint.sh](scripts/find-lint.sh)
(see file for docs) or as part of a build/test/lint cycle using
[scripts/build-test-lint.sh](scripts/build-test-lint.sh).


## HTTP Error Handling

Unfortunately, converting errors into HTTP responses with the correct status
code and message can be done in a number of ways in golang:

1. Having functions return `JSONResponse` directly, which can then either set
   it to an error response or a `200 OK`.
2. Have the HTTP handler try and cast error values to types that are handled
   differently.
3. Have the HTTP handler call functions whose errors can only be interpreted
   one way, for example if a `validate(...)` call returns an error then handler
   knows to respond with a `400 Bad Request`.

We attempt to always use option #3, as it more naturally fits with the way that
golang generally does error handling. In particular, option #1 effectively
requires reinventing a new error handling scheme just for HTTP handlers.


## Line length

We strive for a line length of roughly 80 characters, though less than 100 is
acceptable if necessary. Longer lines are fine if there is nothing of interest
after the first 80-100 characters (e.g. long string literals).


## TODOs and FIXMEs

The majority of TODOs and FIXMEs should have an associated tracking issue on
github. These can be added just before merging of the PR to master, and the
issue number should be added to the comment, e.g. `// TODO(#324): ...`


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
