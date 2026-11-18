#!/usr/bin/env bash
# Checks what claudes reads from the installed Claude Code: `claude agents
# --json`, empty and then with a background job, and that job's state.json.
# The job needs no login: it starts and waits for one.
#
# Usage: .github/workflows/hack/compat-live.sh
# Needs go, jq, and claude on PATH. It trusts a new directory in
# ~/.claude.json and leaves a job running, so it runs only in CI.
set -euo pipefail

if [ "${CI:-}" != true ]; then
  echo "compat-live.sh changes ~/.claude.json and starts a job, so it runs only in CI" >&2
  exit 1
fi

live() {
  CLAUDES_LIVE_CLAUDE=$1 go test ./internal/compat -run '^TestLiveClaude$' -count=1 -v
}

live list

# Claude Code refuses a background job in a directory nobody trusted yet.
work=$(mktemp -d)
config=${CLAUDE_CONFIG_DIR:-$HOME}/.claude.json
[ -s "$config" ] || echo '{}' >"$config"
jq --arg dir "$work" '.hasCompletedOnboarding = true | .projects[$dir].hasTrustDialogAccepted = true' \
  "$config" >"$config.new"
mv "$config.new" "$config"
(cd "$work" && claude --bg "Reply with ok." </dev/null)

live job
