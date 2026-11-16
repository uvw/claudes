#!/usr/bin/env bash
# Formats docs/changelog.md in the open release pull request with Prettier, and
# pushes the result to its branch when it changed. It works in a separate
# worktree, so the checkout this script runs from stays as it is.
#
# A push made with the workflow's own token starts no workflows, so without
# RELEASE_PLEASE_TOKEN (OWN_TOKEN=false) it starts CI on the branch by hand,
# which gives the new commit the check the ruleset requires.
#
# Env: TOKEN (pushes), OWN_TOKEN (true or false), GH_TOKEN (gh),
#      GITHUB_REPOSITORY, and the commit's author as GIT_AUTHOR_* and
#      GIT_COMMITTER_*.
set -euo pipefail

prettier=prettier@3.9.10
: "${TOKEN:?}" "${OWN_TOKEN:?}" "${GITHUB_REPOSITORY:?}"

branch=$(gh pr list --repo "$GITHUB_REPOSITORY" --state open --label "autorelease: pending" \
  --json headRefName -q '.[0].headRefName // empty')
if [[ -z $branch ]]; then
  echo "No open release pull request"
  exit 0
fi

work=$(mktemp -d)
git fetch --quiet origin "refs/heads/$branch"
git worktree add --quiet --detach "$work" FETCH_HEAD
cd "$work"
# The branch can predate the change that stopped .prettierignore from listing
# the changelog, and Prettier skips an ignored file without a word.
npx --yes "$prettier" --ignore-path /dev/null --write docs/changelog.md
if git diff --quiet -- docs/changelog.md; then
  echo "docs/changelog.md in $branch is already formatted"
  exit 0
fi

git commit --quiet -m "chore: format the changelog" -- docs/changelog.md
git push --quiet "https://x-access-token:${TOKEN}@github.com/${GITHUB_REPOSITORY}.git" "HEAD:refs/heads/${branch}"
echo "Pushed the formatted changelog to $branch"

if [[ $OWN_TOKEN != true ]]; then
  gh workflow run ci.yaml --repo "$GITHUB_REPOSITORY" --ref "$branch"
  echo "Started CI on $branch"
fi
