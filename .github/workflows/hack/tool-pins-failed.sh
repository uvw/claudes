#!/usr/bin/env bash
# Reports a failed Tool pins run on the issue labeled tool-pins: it comments on
# the open one, or opens one. A failed run checked no pins, so it would
# otherwise pass without a word. The next run that succeeds updates or closes
# the issue.
#
# Usage: .github/workflows/hack/tool-pins-failed.sh LOG RUN_URL
# Needs gh, with GH_TOKEN and GH_REPO set.
set -euo pipefail

log=${1:?usage: .github/workflows/hack/tool-pins-failed.sh LOG RUN_URL}
run_url=${2:?usage: .github/workflows/hack/tool-pins-failed.sh LOG RUN_URL}
body=$(mktemp)
trap 'rm -f "$body"' EXIT
[ -f "$log" ] || echo "No log: a step before the check failed." >"$log"

# A fence longer than any run of backticks in the log.
fence='```'
while grep -qF -- "$fence" "$log"; do fence="$fence\`"; done

{
  echo "The Tool pins check failed, so no pin was checked against its latest release."
  echo
  echo "${fence}text"
  tail -n 50 "$log"
  echo "$fence"
  echo
  echo "Run: ${run_url}"
} >"$body"

gh label create tool-pins --color c5def5 \
  --description "A tool pinned in the workflows has a newer release" --force
number=$(gh issue list --label tool-pins --state open --json number --jq '.[0].number // empty')
if [ -n "$number" ]; then
  gh issue comment "$number" --body-file "$body"
else
  gh issue create --title "The tool pins check failed" \
    --label tool-pins --body-file "$body"
fi
