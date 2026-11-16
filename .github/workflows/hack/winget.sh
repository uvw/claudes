#!/usr/bin/env bash
# Opens the microsoft/winget-pkgs pull request for a release with komac, from
# the uvw/winget-pkgs fork, with the classic token in WINGET_TOKEN. Without the
# token, or before the package's first version is in winget-pkgs, it does
# nothing: komac update adds a version only to a package that exists, and the
# first goes in by hand (docs/releasing.md).
#
# Usage: .github/workflows/hack/winget.sh TAG
set -euo pipefail

tag=${1:?usage: winget.sh TAG}
version=${tag#v}
id=uvw.claudes
komac_version=2.17.2
komac_sha256=b03b020eec98acfb0b53e284a6e21815b884be33de6785b97e7a120f0e748966

if [[ -z ${WINGET_TOKEN:-} ]]; then
  echo "::notice::WINGET_TOKEN is not set, so no winget pull request was opened"
  exit 0
fi
if ! GH_TOKEN=$WINGET_TOKEN gh api repos/microsoft/winget-pkgs/contents/manifests/u/uvw/claudes >/dev/null 2>&1; then
  echo "::notice::$id is not in winget-pkgs yet; submit its first version by hand (docs/releasing.md)"
  exit 0
fi

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
archive=komac-$komac_version-x86_64-unknown-linux-gnu.tar.gz
curl -fsSL --proto '=https' -o "$work/$archive" \
  "https://github.com/russellbanks/Komac/releases/download/v$komac_version/$archive"
echo "$komac_sha256  $work/$archive" | sha256sum --check --strict
tar -xzf "$work/$archive" -C "$work"
komac=$(find "$work" -type f -name komac -perm -u+x | head -n 1)
[[ -n $komac ]] || {
  echo "::error::$archive holds no komac binary"
  exit 1
}

url=https://github.com/uvw/claudes/releases/download/$tag
GITHUB_TOKEN=$WINGET_TOKEN "$komac" update "$id" --version "$version" \
  --urls "$url/claudes_${version}_windows_amd64.zip" "$url/claudes_${version}_windows_arm64.zip" \
  --submit
