#!/usr/bin/env bash
# Formats docs/changelog.md in the open release pull request with Prettier, in
# a job with no token that can push. When that changes it, the script writes
# the result to OUT/docs/changelog.md and the commit it formatted to OUT/sha,
# for release-pr-push.sh to push. Needs Prettier from tools/package.json
# (npm ci --prefix tools).
#
# It works in a separate worktree, removed on exit, so the checkout it runs
# from stays as it is.
#
# Usage: .github/workflows/hack/format-changelog.sh OUT
# Env: GITHUB_REPOSITORY, and GH_TOKEN (read-only, for gh).
set -euo pipefail

out=${1:?usage: format-changelog.sh OUT}
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
git worktree add --quiet --detach "$work" FETCH_HEAD
cd "$work"
# The branch can predate the change that stopped .prettierignore from listing
# the changelog, and Prettier skips an ignored file without a word.
"$prettier" --ignore-path /dev/null --write docs/changelog.md
if git diff --quiet -- docs/changelog.md; then
  echo "docs/changelog.md in $branch is already formatted"
  exit 0
fi
mkdir -p "$out/docs"
cp docs/changelog.md "$out/docs/changelog.md"
git rev-parse HEAD >"$out/sha"
echo "Formatted docs/changelog.md in $branch"
