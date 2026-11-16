#!/bin/sh
# Installs claudes from its GitHub release on macOS and Linux, or upgrades it.
#
#   curl -fsSL https://github.com/uvw/claudes/releases/latest/download/install.sh | sh
#   curl -fsSL .../install.sh | sh -s -- --version v0.2.0 --dir /usr/local/bin
#
# It downloads the archive for this computer and checks it against the
# release's checksums.txt. When the GitHub CLI is installed and signed in, it
# also checks the archive's build provenance. CLAUDES_INSTALL_DIR and
# CLAUDES_VERSION set the folder and the version, as --dir and --version do.
set -eu

repo=uvw/claudes
dir=${CLAUDES_INSTALL_DIR:-$HOME/.local/bin}
version=${CLAUDES_VERSION:-latest}

say() { printf '%s\n' "$*"; }
die() {
  printf 'claudes install: %s\n' "$*" >&2
  exit 1
}

usage() {
  say "Usage: install.sh [--version vX.Y.Z] [--dir FOLDER]"
  say ""
  say "Installs claudes into FOLDER, ~/.local/bin by default, from the latest"
  say "release or the one --version names. Run it again to upgrade."
}

while [ $# -gt 0 ]; do
  case $1 in
  --version)
    [ $# -ge 2 ] || die "--version needs a tag, such as v0.2.0"
    version=$2
    shift 2
    ;;
  --version=*)
    version=${1#*=}
    shift
    ;;
  --dir)
    [ $# -ge 2 ] || die "--dir needs a folder"
    dir=$2
    shift 2
    ;;
  --dir=*)
    dir=${1#*=}
    shift
    ;;
  -h | --help)
    usage
    exit 0
    ;;
  *) die "unknown option $1 (see --help)" ;;
  esac
done

case $(uname -s) in
Darwin) os=darwin ;;
Linux) os=linux ;;
*) die "$(uname -s) is not supported; on Windows, use install.ps1" ;;
esac
case $(uname -m) in
x86_64 | amd64) arch=amd64 ;;
arm64 | aarch64) arch=arm64 ;;
*) die "$(uname -m) is not supported: releases cover amd64 and arm64" ;;
esac
# A shell under Rosetta reports x86_64 on an Apple silicon Mac.
if [ "$os" = darwin ] && [ "$arch" = amd64 ] && [ "$(sysctl -in sysctl.proc_translated 2>/dev/null)" = 1 ]; then
  arch=arm64
fi

# CLAUDES_DOWNLOAD_URL, a URL or a folder, replaces the release, so the
# release workflow can try this script on the archives it just built.
if [ -n "${CLAUDES_DOWNLOAD_URL:-}" ]; then
  base=$CLAUDES_DOWNLOAD_URL
elif [ "$version" = latest ]; then
  base=https://github.com/$repo/releases/latest/download
else
  case $version in
  v*) ;;
  *) version=v$version ;;
  esac
  printf '%s\n' "$version" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$' || die "$version is not a version such as v0.2.0"
  base=https://github.com/$repo/releases/download/$version
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM

fetch() {
  case $base in
  https://*)
    if command -v curl >/dev/null 2>&1; then
      curl -fsSL --proto '=https' --tlsv1.2 -o "$tmp/$1" "$base/$1" || die "cannot download $base/$1"
    elif command -v wget >/dev/null 2>&1; then
      wget -q --https-only -O "$tmp/$1" "$base/$1" || die "cannot download $base/$1"
    else
      die "neither curl nor wget is installed"
    fi
    ;;
  http://*) die "$base is not https" ;;
  *) cp "$base/$1" "$tmp/$1" || die "cannot copy $base/$1" ;;
  esac
}

fetch checksums.txt
# Each line is "<sha256>  <file>"; the archive name carries the version.
line=$(grep -E "^[0-9a-f]{64}  claudes_[0-9]+\.[0-9]+\.[0-9]+_${os}_${arch}\.tar\.gz\$" "$tmp/checksums.txt" | head -n 1)
[ -n "$line" ] || die "the release has no archive for ${os}_${arch}"
want=${line%% *}
archive=${line##* }
say "Downloading $archive"
fetch "$archive"

if command -v sha256sum >/dev/null 2>&1; then
  got=$(sha256sum "$tmp/$archive" | cut -d ' ' -f 1)
elif command -v shasum >/dev/null 2>&1; then
  got=$(shasum -a 256 "$tmp/$archive" | cut -d ' ' -f 1)
else
  die "neither sha256sum nor shasum is installed, so the download cannot be checked"
fi
[ "$got" = "$want" ] || die "$archive does not match checksums.txt: the download is damaged or was changed"
say "Checksum matches checksums.txt"

if [ "${base#https://github.com/}" = "$base" ]; then
  say "Build provenance not checked: the archive did not come from GitHub"
elif command -v gh >/dev/null 2>&1 && gh auth status >/dev/null 2>&1; then
  gh attestation verify "$tmp/$archive" --repo "$repo" >/dev/null ||
    die "$archive has no valid build provenance from $repo"
  say "Build provenance verified with gh"
else
  say "Build provenance not checked: it needs the GitHub CLI (gh), signed in"
fi

tar -xzf "$tmp/$archive" -C "$tmp" claudes || die "cannot unpack $archive"
mkdir -p "$dir" || die "cannot create $dir"
# Copied beside the old binary, then renamed over it, so a failure leaves the
# old one in place and a running claudes keeps working.
if ! { cp "$tmp/claudes" "$dir/.claudes.new" && chmod 755 "$dir/.claudes.new" &&
  mv -f "$dir/.claudes.new" "$dir/claudes"; }; then
  rm -f "$dir/.claudes.new"
  die "cannot write to $dir"
fi
installed=$("$dir/claudes" --version) || die "the installed $dir/claudes does not run"
say "Installed $installed in $dir"

case ":$PATH:" in
*":$dir:"*) ;;
*)
  say ""
  say "$dir is not on PATH. Add it in your shell's profile:"
  say "  export PATH=\"$dir:\$PATH\""
  ;;
esac

say ""
say "Next steps, each optional:"
case $(basename "${SHELL:-sh}") in
zsh) say "  claudes cd:   add  eval \"\$(claudes init zsh)\"  to ~/.zshrc" ;;
bash) say "  claudes cd:   add  eval \"\$(claudes init bash)\"  to ~/.bashrc" ;;
fish) say "  claudes cd:   add  claudes init fish | source  to ~/.config/fish/config.fish" ;;
esac
say "  completion:   see https://github.com/$repo#shell-setup"
say "  agent skill:  claudes skill --install"
say "  check:        claudes doctor"
