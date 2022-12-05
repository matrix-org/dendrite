#!/usr/bin/env bash

set -e

# Check if release notes have been changed
# Usage ./check-releasenotes.sh path

# require yq
command -v yq >/dev/null 2>&1 || {
    printf >&2 "%s\n" "yq (https://github.com/mikefarah/yq) is not installed. Aborting."
    exit 1
}

# Absolute path of repository
repository=$(git rev-parse --show-toplevel)

# Allow for a specific chart to be passed in as a argument
if [ $# -ge 1 ] && [ -n "$1" ]; then
    root="$1"
    chart_file="${1}/Chart.yaml"
    if [ ! -f "$chart_file" ]; then
        printf >&2 "File %s\n does not exist.\n" "${chart_file}"
        exit 1
    fi

    cd $root

    if [ -z "$DEFAULT_BRANCH" ]; then
      DEFAULT_BRANCH=$(git remote show origin | awk '/HEAD branch/ {print $NF}')
    fi

    CURRENT=$(cat Chart.yaml | yq e '.annotations."artifacthub.io/changes"' -P -)

    if [ "$CURRENT" == "" ] || [ "$CURRENT" == "null" ]; then
      printf >&2 "Changelog annotation has not been set in %s!\n" "$chart_file"
      exit 1
    fi

    DEFAULT_BRANCH=$(git remote show origin | awk '/HEAD branch/ {print $NF}')
    ORIGINAL=$(git show origin/$DEFAULT_BRANCH:./Chart.yaml | yq e '.annotations."artifacthub.io/changes"' -P -)

    if [ "$CURRENT" == "$ORIGINAL" ]; then
      printf >&2 "Changelog annotation has not been updated in %s!\n" "$chart_file"
      exit 1
    fi
else
    printf >&2 "%s\n" "No chart folder has been specified."
    exit 1
fi
