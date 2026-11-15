#!/usr/bin/env bash
# Opens an issue labeled compatibility with a failed claudes doctor report, or
# comments on the open one, so a week of failures stays in one thread.
#
# Usage: .github/workflows/hack/compat-issue.sh REPORT RUN_URL
# Needs gh, with GH_TOKEN and GH_REPO set, and claude on PATH.
set -euo pipefail

report=${1:?usage: .github/workflows/hack/compat-issue.sh REPORT RUN_URL}
run_url=${2:?usage: .github/workflows/hack/compat-issue.sh REPORT RUN_URL}
# claude may be missing when its install was what failed.
version=$({ claude --version 2>/dev/null || true; } | cut -d' ' -f1)
version=${version:-unknown}
body=$(mktemp)
trap 'rm -f "$body"' EXIT
[ -f "$report" ] || echo "No report: a step before claudes doctor failed." >"$report"

# A fence longer than any run of backticks in the report.
fence='```'
while grep -qF -- "$fence" "$report"; do fence="$fence\`"; done

{
  echo "\`claudes doctor --cli-only --strict\` failed against Claude Code ${version}."
  echo
  echo "${fence}text"
  cat "$report"
  echo "$fence"
  echo
  echo "Run: ${run_url}"
} >"$body"

gh label create compatibility --color d93f0b \
  --description "Claude Code changed what claudes reads" --force
number=$(gh issue list --label compatibility --state open --json number --jq '.[0].number // empty')
if [ -n "$number" ]; then
  gh issue comment "$number" --body-file "$body"
else
  gh issue create --title "Claude Code ${version} fails claudes doctor" \
    --label compatibility --body-file "$body"
fi
