#!/usr/bin/env bash

set -e
set -u

THIS_DIR="$( dirname "${0}" )"

REPOS_DIR="${REPOS_DIR:-$HOME/Projects/Personal/gitops}"
BASE_DIR="${REPOS_DIR}/yago"


DESIREDSTATE_ROOT="${BASE_DIR}/tests/assets/4.2.0/desiredstates/concourse-cluster/desiredstate.yaml"
CONFIGURATION_ROOT="${BASE_DIR}/tests/assets/4.2.0/configurations/concourse-cluster/configuration.yaml"


LOG_LEVEL=DEBUG \
go run ./cmd/yago ds validate \
-d $DESIREDSTATE_ROOT \
--configuration-root $CONFIGURATION_ROOT \
-w terraform \
--namespace legacy
