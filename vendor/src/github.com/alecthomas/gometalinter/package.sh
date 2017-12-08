#!/bin/bash -e

# Only build packages for tagged releases
TAG="$(git tag -l --points-at HEAD)"

export CGO_ENABLED=0

GO_VERSION="$(go version | awk '{print $3}' | cut -d. -f1-2)"

if [ "$GO_VERSION" != "go1.9" ]; then
  echo "$0: not packaging; not on Go 1.9"
  exit 0
fi

if echo "$TAG" | grep -q '^v[0-9]\.[0-9]\.[0-9]\(-.*\)?$' && false; then
  echo "$0: not packaging; no tag or tag not in semver format"
  exit 0
fi

LINTERS="
  github.com/alecthomas/gocyclo
  github.com/alexkohler/nakedret
  github.com/client9/misspell/cmd/misspell
  github.com/dnephin/govet
  github.com/GoASTScanner/gas
  github.com/golang/lint/golint
  github.com/gordonklaus/ineffassign
  github.com/jgautheron/goconst/cmd/goconst
  github.com/kisielk/errcheck
  github.com/mdempsky/maligned
  github.com/mdempsky/unconvert
  github.com/mibk/dupl
  github.com/opennota/check/cmd/structcheck
  github.com/opennota/check/cmd/varcheck
  github.com/stripe/safesql
  github.com/tsenart/deadcode
  github.com/walle/lll/cmd/lll
  golang.org/x/tools/cmd/goimports
  golang.org/x/tools/cmd/gotype
  honnef.co/go/tools/cmd/gosimple
  honnef.co/go/tools/cmd/megacheck
  honnef.co/go/tools/cmd/staticcheck
  honnef.co/go/tools/cmd/unused
  mvdan.cc/interfacer
  mvdan.cc/unparam
"

eval "$(go env | FS='' awk '{printf "REAL_%s\n", $0}')"

function install_go_binary() {
  local SRC
  if [ "$GOOS" = "$REAL_GOOS" -a "$GOARCH" = "$REAL_GOARCH" ]; then
    SRC="${GOPATH}/bin"
  else
    SRC="${GOPATH}/bin/${GOOS}_${GOARCH}"
  fi
  install -m 755 "${SRC}/${1}${SUFFIX}" "${2}"
}

function packager() {
  if [ "$GOOS" = "windows" ]; then
    zip -9 -r -o "${1}".zip "${1}"
  else
    tar cvfj "${1}".tar.bz2 "${1}"
  fi
}

rm -rf "${PWD}/dist"

for GOOS in linux darwin windows; do
  SUFFIX=""
  if [ "$GOOS" = "windows" ]; then
    SUFFIX=".exe"
  fi

  for GOARCH in 386 amd64; do
    export GOPATH="${REAL_GOPATH}"
    DEST="${PWD}/dist/gometalinter-${TAG}-${GOOS}-${GOARCH}"
    install -d -m 755 "${DEST}/linters"
    install -m 644 COPYING "${DEST}"
    cat << EOF > "${DEST}/README.txt"
gometalinter is a tool to normalise the output of Go linters.
See https://github.com/alecthomas/gometalinter for more information.

This is a binary distribution of gometalinter ${TAG}.

All binaries must be installed in the PATH for gometalinter to operate correctly.
EOF
    echo "${DEST}"
    export GOOS GOARCH

    go build -i .
    go build -o "${DEST}/gometalinter${SUFFIX}" -ldflags="-X main.Version=${TAG}" .

    export GOPATH="$PWD/_linters"

    go install -v ${LINTERS}
    for LINTER in ${LINTERS}; do
      install_go_binary $(basename ${LINTER}) "${DEST}/linters"
    done

    (cd "${DEST}/.." && packager "$(basename ${DEST})")
  done
done
