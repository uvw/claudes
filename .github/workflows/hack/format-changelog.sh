#!/usr/bin/env bash
# Formats docs/changelog.md in the open release pull request with Prettier, in
# two steps, so Prettier never runs beside a token that can push:
#
#   format OUT  formats the release pull request's changelog and, when that
#               changed it, writes the result to OUT/changelog.md and the
#               commit it formatted to OUT/sha. Needs Prettier from
#               tools/package.json (npm ci --prefix tools).
#   push OUT    commits OUT/changelog.md on top of OUT/sha and pushes it to
#               the release pull request's branch with TOKEN. It pushes
#               nothing when the branch moved since the format step:
#               release-please rewrote it, and the next run formats the new
#               copy.
#
# Each step looks up the pull request itself, so what the format step leaves in
# OUT cannot pick the branch the push step writes to.
#
# Each step works in a separate worktree, removed on exit, so the checkout this
# script runs from stays as it is.
#
# With RELEASE_PLEASE_TOKEN as TOKEN, the push starts CI like any other. With
# the workflow's own token it starts none, and the pull request's checks wait
# for a maintainer's approval (docs/releasing.md).
#
# Env: GITHUB_REPOSITORY, GH_TOKEN (read-only, for gh), and for push, TOKEN
#      and the commit's author as GIT_AUTHOR_* and GIT_COMMITTER_*.
set -euo pipefail

usage="usage: format-changelog.sh format|push OUT"
mode=${1:?$usage}
out=${2:?$usage}
[[ $mode == format || $mode == push ]] || {
  echo "$usage" >&2
  exit 2
}
mkdir -p "$out"
out=$(cd "$out" && pwd)
: "${GITHUB_REPOSITORY:?}"
root=$PWD
prettier=$root/tools/node_modules/.bin/prettier

work=$(mktemp -d)
cleanup() {
  git -C "$root" worktree remove --force "$work" 2>/dev/null || rm -rf "$work"
}
trap cleanup EXIT

branch=$(gh pr list --repo "$GITHUB_REPOSITORY" --state open --label "autorelease: pending" \
  --json headRefName -q '.[0].headRefName // empty')
if [[ -z $branch ]]; then
  echo "No open release pull request"
  exit 0
fi
git fetch --quiet origin "refs/heads/$branch"

case $mode in
format)
  git worktree add --quiet --detach "$work" FETCH_HEAD
  cd "$work"
  # The branch can predate the change that stopped .prettierignore from listing
  # the changelog, and Prettier skips an ignored file without a word.
  "$prettier" --ignore-path /dev/null --write docs/changelog.md
  if git diff --quiet -- docs/changelog.md; then
    echo "docs/changelog.md in $branch is already formatted"
    exit 0
  fi
  cp docs/changelog.md "$out/changelog.md"
  git rev-parse HEAD >"$out/sha"
  echo "Formatted docs/changelog.md in $branch"
  ;;
push)
  : "${TOKEN:?}"
  sha=$(<"$out/sha")
  if [[ $(git rev-parse FETCH_HEAD) != "$sha" ]]; then
    echo "$branch moved since it was formatted, so nothing was pushed"
    exit 0
  fi
  git worktree add --quiet --detach "$work" "$sha"
  cp "$out/changelog.md" "$work/docs/changelog.md"
  cd "$work"
  git commit --quiet -m "chore: format the changelog" -- docs/changelog.md
  git push --quiet "https://x-access-token:${TOKEN}@github.com/${GITHUB_REPOSITORY}.git" "HEAD:refs/heads/${branch}"
  echo "Pushed the formatted changelog to $branch"
  ;;
esac
