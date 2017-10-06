# Code Style

We follow the standard Go style using gofmt, but with a few extra
considerations.

## Linters

We use `gometalinter` to run a number of linters, the exact list can be found
in [linter.json](linter.json). Some of these are slow and expensive to run, but
a subset can be found in [linter-fast.json](linter-fast.json) that run quickly
enough to be run as part of an IDE.

For rare cases where a linter is giving a spurious warning, it can be disabled
for that line or statement using a [comment directive](https://github.com/alecthomas/gometalinter#comment-directives), e.g.
`// nolint: gocyclo`. This should be used sparingly and only when its clear
that the lint warning is spurious.


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


## Visual Studio Code

If you use VSCode then the following is an example of a workspace setting that
sets up linting correctly:

```json
{
    "go.gopath": "${workspaceRoot}:${workspaceRoot}/vendor",
    "go.lintOnSave": "workspace",
    "go.lintTool": "gometalinter",
    "go.lintFlags": ["--config=linter-fast.json", "--concurrency=5"]
}
```
