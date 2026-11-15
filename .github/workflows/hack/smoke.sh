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

"$bin" --version | grep -q '^claudes version '
if [ -n "$want" ]; then
  "$bin" --version | grep -q " ${want#v} " || {
    echo "::error::the binary does not report version ${want#v}"
    exit 1
  }
fi
"$bin" --help | grep -q 'claudes lists, pins, restarts'
"$bin" restart --help | grep -q -- '--ping'

"$bin" skill | head -n 2 | grep -q '^name: claudes$'
"$bin" skill --dir "$work/skills" >/dev/null
cmp <("$bin" skill) "$work/skills/claudes/SKILL.md"

for shell in bash zsh fish powershell pwsh; do
  "$bin" completion "$shell" | grep -q 'claudes'
done
for shell in bash zsh pwsh; do
  "$bin" init "$shell" | grep -q 'session where --one'
done

echo "smoke: $("$bin" --version) passed"
