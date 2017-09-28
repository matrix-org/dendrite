#!/usr/bin/env bash

set -e

LAST=$(grep '^# [0-9]' CHANGELOG.md | head -n1 | cut -d' ' -f2)

{
    echo Changelog
    echo =========
    echo

    echo '# <INSERT NEXT VERSION HERE>'
    echo
    echo "Merged pull requests since ${LAST}:"
    echo

    # -e '/Merge pull request/d' \

    git log --first-parent "v${LAST}.." | grep -A4 '^Date:' | sed \
        -e '/^ *$/d' \
        -e '/^Date:/d' \
        -e '/^commit /d' \
        -e '/^--/d' \
        -e '/Merge pull request/d' \
        -e '/^    / s/^    /* /'

    # TODO: augmenting with PR links should be straight forward, and useful to
    # the reviewer
    #     -e '/Merge pull request/ { s~^.*#\([0-9][0-9]*\).*$~https://github.com/uber/tchannel-go/pull/\1~; h; d; }' \
    #     -e '/^    / { s/^    /* /; G; }'

    echo
    echo '# -- NEW ABOVE, OLD BELOW --'
    echo

    tail -n+4 CHANGELOG.md
} > CHANGELOG.md.new
mv -vf CHANGELOG.md.new CHANGELOG.md
