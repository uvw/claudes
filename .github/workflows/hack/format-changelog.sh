#!/usr/bin/env bash
# Formats docs/changelog.md in the release pull request's checkout with
# Prettier, and pushes the result to BRANCH when it changed.
#
# A push made with the workflow's own token starts no workflows, so without
# RELEASE_PLEASE_TOKEN (OWN_TOKEN=false) it starts CI on the branch by hand,
# which gives the new commit the check the ruleset requires.
#
# Env: BRANCH, TOKEN (pushes), OWN_TOKEN (true or false), GH_TOKEN (gh),
#      GITHUB_REPOSITORY.
set -euo pipefail

prettier=prettier@3.9.10
: "${BRANCH:?}" "${TOKEN:?}" "${OWN_TOKEN:?}" "${GITHUB_REPOSITORY:?}"

npx --yes "$prettier" --write docs/changelog.md
if git diff --quiet -- docs/changelog.md; then
  echo "docs/changelog.md is already formatted"
  exit 0
fi

git -c user.name='github-actions[bot]' \
  -c user.email='41898282+github-actions[bot]@users.noreply.github.com' \
  commit --quiet -m "chore: format the changelog" -- docs/changelog.md
git push --quiet "https://x-access-token:${TOKEN}@github.com/${GITHUB_REPOSITORY}.git" "HEAD:refs/heads/${BRANCH}"
echo "Pushed the formatted changelog to $BRANCH"

if [[ $OWN_TOKEN != true ]]; then
  gh workflow run ci.yaml --repo "$GITHUB_REPOSITORY" --ref "$BRANCH"
  echo "Started CI on $BRANCH"
fi
