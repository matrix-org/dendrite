export GOPATH="$(pwd):$(pwd)/vendor"

# Install the linter to the local GOPATH
go get github.com/golangci/golangci-lint/cmd/golangci-lint

# Run the linter
golangci-lint run --max-same-issues 0 --max-issues-per-linter 0
