#!/usr/bin/env bash
# Writes the Homebrew formula for a release from its checksums.txt, and pushes
# it to uvw/homebrew-tap with the token in HOMEBREW_TAP_TOKEN. Without the
# token it prints the formula and pushes nothing.
#
# Usage: .github/workflows/hack/homebrew.sh TAG CHECKSUMS
set -euo pipefail

tag=${1:?usage: homebrew.sh TAG CHECKSUMS}
sums=${2:?usage: homebrew.sh TAG CHECKSUMS}
version=${tag#v}
tap=uvw/homebrew-tap
url=https://github.com/uvw/claudes/releases/download/$tag

sha() {
  local sum
  sum=$(awk -v f="claudes_${version}_$1.tar.gz" '$2 == f { print $1 }' "$sums")
  [[ $sum =~ ^[0-9a-f]{64}$ ]] || {
    echo "::error::$sums has no claudes_${version}_$1.tar.gz"
    exit 1
  }
  printf '%s' "$sum"
}

# Each in its own assignment, so a missing archive stops the script.
mac_arm=$(sha darwin_arm64)
mac_intel=$(sha darwin_amd64)
linux_arm=$(sha linux_arm64)
linux_intel=$(sha linux_amd64)

formula=$(
  cat <<EOF
class Claudes < Formula
  desc "Manage Claude Code sessions: list, pin, restart, pause, and resume"
  homepage "https://github.com/uvw/claudes"
  version "$version"
  license "MIT"

  on_macos do
    on_arm do
      url "$url/claudes_${version}_darwin_arm64.tar.gz"
      sha256 "$mac_arm"
    end
    on_intel do
      url "$url/claudes_${version}_darwin_amd64.tar.gz"
      sha256 "$mac_intel"
    end
  end

  on_linux do
    on_arm do
      url "$url/claudes_${version}_linux_arm64.tar.gz"
      sha256 "$linux_arm"
    end
    on_intel do
      url "$url/claudes_${version}_linux_amd64.tar.gz"
      sha256 "$linux_intel"
    end
  end

  def install
    bin.install "claudes"
    generate_completions_from_executable(bin/"claudes", "completion")
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/claudes --version")
  end
end
EOF
)

if [[ -z ${HOMEBREW_TAP_TOKEN:-} ]]; then
  echo "::notice::HOMEBREW_TAP_TOKEN is not set, so the formula was not pushed to $tap"
  printf '%s\n' "$formula"
  exit 0
fi

path=Formula/claudes.rb
# The contents API replaces a file only when given the blob it replaces.
current=$(GH_TOKEN=$HOMEBREW_TAP_TOKEN gh api "repos/$tap/contents/$path" -q .sha 2>/dev/null) || current=
args=(-X PUT "repos/$tap/contents/$path" -f "message=claudes $version"
  -f "content=$(printf '%s\n' "$formula" | base64 | tr -d '\n')")
[[ -z $current ]] || args+=(-f "sha=$current")
GH_TOKEN=$HOMEBREW_TAP_TOKEN gh api "${args[@]}" >/dev/null
echo "Pushed the claudes $version formula to $tap"
