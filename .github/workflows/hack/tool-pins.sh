#!/usr/bin/env bash
# Checks the tool versions pinned in the workflows, which Dependabot cannot
# see, against each tool's latest GitHub release. It keeps one issue labeled
# tool-pins open with the list of pins that are behind, and closes it once none
# are.
#
# The pins are read from the files that hold them, so those files stay the one
# place each version is written.
#
# Usage: .github/workflows/hack/tool-pins.sh [--dry-run] [RUN_URL]
# Needs gh, with GH_TOKEN and GH_REPO set. With --dry-run, it prints the issue
# body and what it would do, and changes nothing.
set -euo pipefail

dry_run=false
if [[ ${1:-} == --dry-run ]]; then
  dry_run=true
  shift
fi
run_url=${1:-}
cd "$(dirname "$0")/../../.."

# Each pin: the tool, its GitHub repository, the key that holds the version,
# the action whose step holds that key (or - for a key anywhere in the file),
# and a checksum pinned beside the version (or -).
pins=(
  "goreleaser goreleaser/goreleaser GORELEASER_VERSION - -"
  "golangci-lint golangci/golangci-lint version golangci/golangci-lint-action -"
  "syft anchore/syft syft-version anchore/sbom-action/download-syft -"
  "zizmor zizmorcore/zizmor version zizmorcore/zizmor-action -"
  "gitleaks gitleaks/gitleaks GITLEAKS_VERSION - GITLEAKS_SHA256"
  "komac russellbanks/Komac komac_version - komac_sha256"
)

# Prints FILE:LINE:VERSION for each place KEY is set, in a step that uses
# ACTION unless ACTION is -.
find_pin() {
  awk -v key="$1" -v action="$2" '
    FNR == 1 { inside = (action == "-") }
    action != "-" && index($0, "uses: " action "@") { inside = 1; next }
    action != "-" && /^[[:space:]]*- / { inside = 0 }
    inside && match($0, "^[[:space:]]*" key "[:=][[:space:]]*") {
      value = substr($0, RSTART + RLENGTH)
      sub(/[[:space:]#].*/, "", value)
      gsub(/["\047]/, "", value)
      print FILENAME ":" FNR ":" value
    }' .github/workflows/*.yaml .github/workflows/hack/*.sh
}

# True when version $1 is older than version $2, ignoring a leading v.
older() {
  local a=${1#v} b=${2#v}
  [[ $a != "$b" && $(printf '%s\n%s\n' "$a" "$b" | sort -V | head -n 1) == "$a" ]]
}

behind=()
missing=()
for pin in "${pins[@]}"; do
  read -r tool repo key action checksum <<<"$pin"
  found=$(find_pin "$key" "$action")
  if [[ -z $found ]]; then
    missing+=("$tool ($key)")
    continue
  fi
  latest=$(gh api "repos/$repo/releases/latest" --jq .tag_name)
  while IFS=: read -r file line current; do
    echo "$tool $current at $file:$line, latest $latest"
    if older "$current" "$latest"; then
      # Written the way the pin is, with or without the v.
      wanted=${latest#v}
      [[ $current != v* ]] || wanted=v$wanted
      where="\`$file:$line\`"
      [[ $checksum == - ]] || where="$where, with \`$checksum\`"
      behind+=("| [$tool](https://github.com/$repo/releases/latest) | $current | $wanted | $where |")
    fi
  done <<<"$found"
done

# A pin that moved or changed shape would otherwise drop out of the check.
if ((${#missing[@]})); then
  echo "::error::Found no pin for ${missing[*]}: update the pins list in $0"
  exit 1
fi

number=$(gh issue list --label tool-pins --state open --json number --jq '.[0].number // empty')

if ((${#behind[@]} == 0)); then
  echo "Every tool pin is at its latest release"
  if [[ -n $number ]]; then
    if $dry_run; then
      echo "Would close issue #$number"
    else
      gh issue close "$number" --comment "Every tool pin is at its latest release.${run_url:+ Run: $run_url}"
    fi
  fi
  exit 0
fi

body=$(mktemp)
trap 'rm -f "$body"' EXIT
{
  echo "These pins are behind their tool's latest release, and Dependabot does not"
  echo "update them. Raise each one where it lives. Where a checksum is listed, take"
  echo "its new value from the release's checksums file."
  echo
  echo "| Tool | Pinned | Latest | Where |"
  echo "| ---- | ------ | ------ | ----- |"
  printf '%s\n' "${behind[@]}"
  if [[ -n $run_url ]]; then
    echo
    echo "Run: $run_url"
  fi
} >"$body"

if $dry_run; then
  echo
  cat "$body"
  echo
  if [[ -n $number ]]; then
    echo "Would update issue #$number"
  else
    echo "Would open an issue"
  fi
  exit 0
fi

gh label create tool-pins --color c5def5 \
  --description "A tool pinned in the workflows has a newer release" --force
if [[ -n $number ]]; then
  gh issue edit "$number" --body-file "$body"
else
  gh issue create --title "Tool pins are behind their latest releases" \
    --label tool-pins --body-file "$body"
fi
