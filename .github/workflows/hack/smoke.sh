#!/usr/bin/env bash
# Runs a built claudes through the commands that need no Claude Code: version,
# help, the skill, and shell completion.
#
# Usage: .github/workflows/hack/smoke.sh BINARY
#        .github/workflows/hack/smoke.sh ARCHIVE [VERSION]
#
# Given a release archive (.tar.gz or .zip), it unpacks it first. With VERSION,
# it also checks that the binary reports that version.
set -euo pipefail

target=${1:?usage: .github/workflows/hack/smoke.sh BINARY | ARCHIVE [VERSION]}
want=${2:-}
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

case "$target" in
*.zip)
  unzip -q "$target" -d "$work/unpacked"
  bin="$work/unpacked/claudes.exe"
  ;;
*.tar.gz)
  mkdir "$work/unpacked"
  tar -xzf "$target" -C "$work/unpacked"
  bin="$work/unpacked/claudes"
  ;;
*) bin=$target ;;
esac

# has PATTERN COMMAND... runs the command, then searches its whole output. A
# grep -q in a pipe exits at the first match, and the command then fails with
# SIGPIPE under pipefail.
has() {
  local pattern=$1 out
  shift
  out=$("$@")
  grep -q -- "$pattern" <<<"$out"
}

has '^claudes version ' "$bin" --version
if [ -n "$want" ]; then
  has " ${want#v} " "$bin" --version || {
    echo "::error::the binary does not report version ${want#v}"
    exit 1
  }
fi
has 'claudes lists, pins, restarts' "$bin" --help
has '--ping' "$bin" restart --help

skill=$("$bin" skill)
[ "$(sed -n 2p <<<"$skill")" = "name: claudes" ]
"$bin" skill --dir "$work/skills" >/dev/null
cmp <(printf '%s\n' "$skill") "$work/skills/claudes/SKILL.md"

for shell in bash zsh fish powershell pwsh; do
  has 'claudes' "$bin" completion "$shell"
done
for shell in bash zsh pwsh; do
  has 'session where --one' "$bin" init "$shell"
done

echo "smoke: $("$bin" --version) passed"
