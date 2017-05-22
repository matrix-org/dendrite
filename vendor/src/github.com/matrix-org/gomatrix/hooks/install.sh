#! /bin/bash

DOT_GIT="$(dirname $0)/../.git"

ln -s "../../hooks/pre-commit" "$DOT_GIT/hooks/pre-commit"