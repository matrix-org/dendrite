#!/usr/bin/env bash
set -eu

# Generate helm-docs for Helm charts
# Usage ./gen-helm-docs.sh [stable/incubator] [chart]

# require helm-docs
command -v helm-docs >/dev/null 2>&1 || {
    echo >&2 "helm-docs (https://github.com/k8s-at-home/helm-docs) is not installed. Aborting."
    exit 1
}

# Absolute path of repository
repository=$(git rev-parse --show-toplevel)

# Templates to copy into each chart directory
readme_template="${repository}/hack/templates/README.md.gotmpl"
readme_config_template="${repository}/hack/templates/README_CONFIG.md.gotmpl"

# Gather all charts using the common library, excluding common-test
charts=$(find "${repository}" -name "Chart.yaml")

# Allow for a specific chart to be passed in as a argument
if [ $# -ge 1 ] && [ -n "$1" ] && [ -n "$2" ]; then
    charts="${repository}/charts/$1/$2/Chart.yaml"
    root="$(dirname "${charts}")"
    if [ ! -f "$charts" ]; then
        echo "File ${charts} does not exist."
        exit 1
    fi
else
    root="${repository}/charts/stable"
fi

for chart in ${charts}; do
    chart_directory="$(dirname "${chart}")"
    echo "-] Copying templates to ${chart_directory}"
    # Copy CONFIG template to each Chart directory, do not overwrite if exists
    cp -n "${readme_config_template}" "${chart_directory}" || true
done

# Run helm-docs for charts using the common library and the common library itself
helm-docs \
    --ignore-file="${repository}/.helmdocsignore" \
    --template-files="${readme_template}" \
    --template-files="$(basename "${readme_config_template}")" \
    --chart-search-root="${root}"
