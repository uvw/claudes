#!/usr/bin/env bash
# Runs every fuzz target in the repository for FUZZTIME each, one package per
# go test, and keeps going after a failure, so one run reports every target
# that fails. Each failure's output goes into REPORT, and go test writes its
# input under the package's testdata/fuzz.
#
# Usage: .github/workflows/hack/fuzz.sh FUZZTIME REPORT
set -euo pipefail

fuzztime=${1:?usage: .github/workflows/hack/fuzz.sh FUZZTIME REPORT}
report=${2:?usage: .github/workflows/hack/fuzz.sh FUZZTIME REPORT}
log=$(mktemp)
trap 'rm -f "$log"' EXIT
: >"$report"

failed=0
targets=$(git grep -E '^func Fuzz[A-Za-z0-9_]*\(' -- '*_test.go' |
  sed -E 's|^(.*)/[^/]*:func (Fuzz[A-Za-z0-9_]*)\(.*|./\1 \2|' | sort)
[ -n "$targets" ] || {
  echo "no fuzz targets found" >&2
  exit 1
}
while read -r pkg name; do
  echo "::group::$name in $pkg"
  if ! go test "$pkg" -run '^$' -fuzz "^${name}\$" -fuzztime "$fuzztime" 2>&1 | tee "$log"; then
    failed=1
    {
      echo "$name in $pkg:"
      echo
      echo '```text'
      tail -n 60 "$log"
      echo '```'
      echo
    } >>"$report"
  fi
  echo "::endgroup::"
done <<<"$targets"
exit "$failed"
