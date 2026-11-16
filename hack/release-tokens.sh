#!/usr/bin/env bash
# Sets up what the release workflow needs to publish to Homebrew and winget:
# the uvw/homebrew-tap repository, a fork of microsoft/winget-pkgs, and two
# tokens saved as secrets on uvw/claudes.
#
# GitHub creates tokens only in the browser, so the script opens each token
# page with its fields filled in, then reads the token you paste. It never
# prints a token or passes one on a command line.
#
# Usage: hack/release-tokens.sh
set -euo pipefail

repo=uvw/claudes
tap=uvw/homebrew-tap

say() { printf '%s\n' "$*"; }
die() {
  printf 'release-tokens: %s\n' "$*" >&2
  exit 1
}

open_url() {
  say "  $1"
  if command -v open >/dev/null 2>&1; then
    open "$1"
  elif command -v xdg-open >/dev/null 2>&1; then
    xdg-open "$1" >/dev/null 2>&1
  fi
}

# read_token NAME reads a token without echoing it, into the variable token.
read_token() {
  token=
  while [[ -z $token ]]; do
    read -rsp "Paste the $1 token (it is not shown): " token </dev/tty
    printf '\n'
  done
}

command -v gh >/dev/null 2>&1 || die "the GitHub CLI (gh) is not installed"
gh auth status >/dev/null 2>&1 || die "gh is not signed in: run gh auth login"

say "1. Repositories"
if gh repo view "$tap" >/dev/null 2>&1; then
  say "   $tap exists"
else
  gh repo create "$tap" --public --description "Homebrew tap for claudes" --add-readme >/dev/null
  say "   created $tap"
fi
if gh repo view uvw/winget-pkgs >/dev/null 2>&1; then
  say "   uvw/winget-pkgs exists"
else
  gh repo fork microsoft/winget-pkgs --clone=false --default-branch-only >/dev/null
  say "   forked microsoft/winget-pkgs to uvw/winget-pkgs"
fi

say ""
say "2. HOMEBREW_TAP_TOKEN: a fine-grained token that can push to $tap"
say "   The page opens with the name, owner, expiry, and Contents: write filled in."
say "   Under Repository access, choose \"Only select repositories\" and pick"
say "   $tap, then Generate token and copy it."
open_url "https://github.com/settings/personal-access-tokens/new?name=claudes+homebrew-tap&description=Release+workflow+pushes+the+claudes+formula+to+$tap&target_name=uvw&expires_in=366&contents=write"
read_token "Homebrew tap"
push=$(GH_TOKEN=$token gh api "repos/$tap" -q .permissions.push 2>/dev/null) || push=
[[ $push == true ]] || die "that token cannot push to $tap; check its repository access and Contents permission"
printf '%s' "$token" | gh secret set HOMEBREW_TAP_TOKEN -R "$repo"
say "   saved HOMEBREW_TAP_TOKEN on $repo"

say ""
say "3. WINGET_TOKEN: a classic token with the public_repo and workflow scopes"
say "   winget's tools open pull requests on microsoft/winget-pkgs from your fork,"
say "   which only a classic token can do. Check that public_repo and workflow"
say "   are ticked, set an expiry, then Generate token and copy it."
open_url "https://github.com/settings/tokens/new?scopes=public_repo,workflow&description=claudes+winget+releases"
read_token "winget"
scopes=$(GH_TOKEN=$token gh api -i user 2>/dev/null | tr -d '\r' | sed -n 's/^[Xx]-[Oo][Aa]uth-[Ss]copes: //p') || scopes=
[[ ",${scopes// /}," == *,public_repo,* ]] || die "that token lacks the public_repo scope (it has: ${scopes:-none})"
printf '%s' "$token" | gh secret set WINGET_TOKEN -R "$repo"
say "   saved WINGET_TOKEN on $repo"
unset token

say ""
say "Done. The release workflow can now publish to $tap and open winget pull requests."
