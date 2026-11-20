#!/usr/bin/env bash
# Opens an issue labeled fuzz with the targets a weekly fuzz run failed, or
# comments on the open one, so the failures stay in one thread.
#
# Usage: .github/workflows/hack/fuzz-issue.sh REPORT RUN_URL
# Needs gh, with GH_TOKEN and GH_REPO set.
set -euo pipefail

report=${1:?usage: .github/workflows/hack/fuzz-issue.sh REPORT RUN_URL}
run_url=${2:?usage: .github/workflows/hack/fuzz-issue.sh REPORT RUN_URL}
body=$(mktemp)
trap 'rm -f "$body"' EXIT
[ -s "$report" ] || echo "No report: a step before the fuzzing failed." >"$report"

{
  echo "The weekly fuzz run failed."
  echo
  cat "$report"
  echo "The run's fuzz-inputs artifact holds each failing input, under its package's \`testdata/fuzz/<target>\`. Copy it to the same path, and \`go test\` on that package runs it."
  echo
  echo "Run: ${run_url}"
} >"$body"

gh label create fuzz --color 5319e7 \
  --description "A fuzz target found a failing input" --force
number=$(gh issue list --label fuzz --state open --json number --jq '.[0].number // empty')
if [ -n "$number" ]; then
  gh issue comment "$number" --body-file "$body"
else
  gh issue create --title "The weekly fuzz run found a failing input" \
    --label fuzz --body-file "$body"
fi
