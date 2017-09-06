This directory looks a bit like a normal vendor directory, but also like an
entry in GOPATH. That is not an accident. It looks like the former so that gvt
can be used to manage the vendored linters, and it looks like a GOPATH entry so
that we can install the vendored binaries (as Go does not support installing
binaries from vendored paths).
