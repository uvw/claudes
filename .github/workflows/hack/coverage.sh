#!/usr/bin/env bash
# Fails when total test coverage is under FLOOR percent.
# Usage: .github/workflows/hack/coverage.sh FLOOR [PROFILE], after
# go test -coverprofile=PROFILE ./... (PROFILE defaults to coverage.out).
set -euo pipefail

floor=${1:?usage: .github/workflows/hack/coverage.sh FLOOR [PROFILE]}
profile=${2:-coverage.out}

total=$(go tool cover -func="$profile" | awk '/^total:/ { sub("%", "", $3); print $3 }')
echo "Total coverage: ${total}% (floor ${floor}%)"
awk -v t="$total" -v f="$floor" 'BEGIN { exit !(t + 0 >= f + 0) }' || {
  echo "::error::coverage ${total}% is under the ${floor}% floor"
  exit 1
}
