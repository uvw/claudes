#!/usr/bin/env bash
# Tries install/install.sh on a release directory: one made from a built binary,
# or the release workflow's dist/. It installs twice, as a fresh install and
# an upgrade, then checks that a damaged archive is refused.
#
# Usage: .github/workflows/hack/install-test.sh BINARY
#        .github/workflows/hack/install-test.sh --release DIST [VERSION]
set -euo pipefail

root=$(cd "$(dirname "$0")/../../.." && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

case "$(uname -s)" in Darwin) os=darwin ;; *) os=linux ;; esac
case "$(uname -m)" in x86_64 | amd64) arch=amd64 ;; *) arch=arm64 ;; esac

if [[ ${1:-} == --release ]]; then
  release=${2:?usage: install-test.sh --release DIST [VERSION]}
  want=${3:-}
else
  binary=${1:?usage: install-test.sh BINARY}
  release=$work/release
  mkdir -p "$release" "$work/pack"
  cp "$binary" "$work/pack/claudes"
  archive=claudes_0.0.0_${os}_${arch}.tar.gz
  tar -czf "$release/$archive" -C "$work/pack" claudes
  (cd "$release" && { sha256sum "$archive" 2>/dev/null || shasum -a 256 "$archive"; } >checksums.txt)
  want=
fi

for run in fresh upgrade; do
  out=$(CLAUDES_DOWNLOAD_URL=$release sh "$root/install/install.sh" --dir "$work/bin")
  grep -q '^Checksum matches checksums.txt$' <<<"$out" || {
    echo "$run: no checksum line:"
    echo "$out"
    exit 1
  }
  version=$("$work/bin/claudes" --version)
  [[ -z $want || $version == *"${want#v}"* ]] || {
    echo "$run: installed $version, want $want"
    exit 1
  }
  echo "$run install: $version"
done

damaged=$work/damaged
cp -R "$release" "$damaged"
for f in "$damaged"/claudes_*_"${os}_${arch}".tar.gz; do printf 'x' >>"$f"; done
if CLAUDES_DOWNLOAD_URL=$damaged sh "$root/install/install.sh" --dir "$work/bad" 2>"$work/err"; then
  echo "a damaged archive was installed"
  exit 1
fi
grep -q 'does not match checksums.txt' "$work/err" || {
  cat "$work/err"
  exit 1
}
[[ ! -e $work/bad/claudes ]] || {
  echo "a damaged archive left a binary behind"
  exit 1
}
echo "damaged archive refused"
