# Releasing claudes

Releases are automated. Merging a pull request into `main` is the only manual
step.

## How a release happens

1. Pull requests squash-merge, so each PR title becomes one Conventional Commit
   on `main`. CI's "Pull request title" job enforces the format, and CI runs
   again when the title changes. Dependabot's titles, such as
   `fix(deps): Bump charm.land/lipgloss/v2 from 2.0.6 to 2.1.0`, pass with their
   capital "Bump". A `fix(deps)` update lands in the changelog's "Bug fixes" as
   **deps:** and makes a patch release, since it changes the binary. A
   `ci(deps)` update makes none.
2. On every push to `main`, `.github/workflows/release-please.yaml` updates an
   open pull request titled `chore(main): release X.Y.Z`. It holds the next
   version and the `docs/changelog.md` entry, worked out from those commits:
   `feat` bumps the minor version and `fix` the patch while the version is below
   1.0. release-please writes the entry with long lines, so the same workflow
   then formats `docs/changelog.md` in that pull request with Prettier
   (`.github/workflows/hack/format-changelog.sh`), in a job with no token that
   can push. A later job pushes the result with `RELEASE_PLEASE_TOKEN`, so the
   pull request's checks start without approval.
3. Merging that pull request makes release-please create the `vX.Y.Z` tag and a
   draft release with the changelog as its notes. It then calls
   `.github/workflows/release.yaml` with the tag.
4. `release.yaml` runs, in order:
   1. **Eligibility:** the tag is `vX.Y.Z`, with no prerelease part, and is on
      `main`. Every later job builds the commit the tag named here, and publish
      checks that the tag still names it, so moving the tag mid-run fails the
      release.
   2. **CI:** the whole of `ci.yaml` on the tag.
   3. **Build:** goreleaser builds every archive, `checksums.txt`, and one SBOM
      per archive, but publishes nothing.
   4. **Smoke:** all six archives, each on a runner of its own OS and
      architecture, are unpacked and run through
      `.github/workflows/hack/smoke.sh`, which checks that `--version` reports
      the tag, then installed with their install script.
   5. **Attest:** build provenance for every file in `checksums.txt`.
   6. **Publish:** uploads the assets to the draft, checks the tag again, and
      publishes it.

Releases are immutable once published, so a broken release is fixed by releasing
the next version, never by editing it.

## Cutting a release

1. `gh pr list --search "release in:title"` finds the release pull request.
2. Read its `docs/changelog.md` diff. If a commit landed with the wrong type,
   fix it with `Release-As: X.Y.Z` in the body of an empty commit, or edit the
   pull request's changelog before merging.
3. Merge it once CI is green: `gh pr merge --squash --auto <number>`.
4. Watch the release: `gh run watch` on the newest "Release Please" run.

## Checking a release

```bash
gh release view vX.Y.Z
gh release download vX.Y.Z --pattern '*linux_amd64.tar.gz' --pattern checksums.txt
sha256sum --check --ignore-missing checksums.txt
gh attestation verify claudes_X.Y.Z_linux_amd64.tar.gz --repo uvw/claudes
```

Each release also carries `install.sh` and `install.ps1` from `install/`, listed
in `checksums.txt` and attested with the archives. The smoke jobs install each
platform's archive with its script before anything is published. Check the
one-liners once it is out:

```bash
curl -fsSL https://github.com/uvw/claudes/releases/latest/download/install.sh |
  sh -s -- --dir "$(mktemp -d)"
```

## Package managers

Once a release is published, two jobs in the release workflow update the package
managers. Each is skipped with a notice when its secret is missing.

1. **Homebrew:** `.github/workflows/hack/homebrew.sh` writes
   `Formula/claudes.rb` from `checksums.txt`, for macOS and Linux on Intel and
   ARM, and pushes it to [uvw/homebrew-tap](https://github.com/uvw/homebrew-tap)
   with `HOMEBREW_TAP_TOKEN`, a fine-grained token that can write that
   repository's contents.
2. **winget:** `.github/workflows/hack/winget.sh` runs
   [komac](https://github.com/russellbanks/Komac), pinned by version and
   checksum, to open a pull request on microsoft/winget-pkgs from the
   uvw/winget-pkgs fork, with `WINGET_TOKEN`. That must be a classic token with
   `public_repo`: a fine-grained one cannot open pull requests on another
   owner's public repository.

The weekly Tool pins workflow opens an issue when komac, goreleaser, or syft has
a release newer than its pin ([contributing.md](contributing.md#checks)).

`hack/release-tokens.sh` sets both up: it creates the tap and the fork if they
are missing, opens each token's page with its fields filled in, checks the token
you paste, and saves it as a secret of the `release` environment
([Repository settings](#repository-settings)). When a token expires, run it
again with the secret's name, such as `hack/release-tokens.sh WINGET_TOKEN`.

komac adds versions only to a package winget already has, so the first version
of `uvw.claudes` is submitted by hand, on any machine with komac:

```bash
GITHUB_TOKEN=<the winget token> komac new uvw.claudes --version X.Y.Z \
  --urls https://github.com/uvw/claudes/releases/download/vX.Y.Z/claudes_X.Y.Z_windows_amd64.zip \
         https://github.com/uvw/claudes/releases/download/vX.Y.Z/claudes_X.Y.Z_windows_arm64.zip
```

komac asks for the rest: publisher `uvw`, package name `claudes`, license MIT,
and the portable `claudes.exe` inside each zip, with the command alias
`claudes`. Microsoft reviews the first pull request by hand; the release job
handles every version after it.

## When it fails

- **A workflow failed before publish:** the draft and the tag exist, and nothing
  is published. Fix the workflow on `main` through a pull request, then rerun
  with `gh workflow run release.yaml -f tag=vX.Y.Z`. A rerun takes the workflows
  from `main` but everything else from the tag, such as `go.mod`,
  `.goreleaser.yaml`, and the scripts under `.github/workflows/hack/`. The
  publish job uploads with `--clobber`, so it replaces any partial upload.
- **The fix is in the tagged code, or the release should wait:** delete the
  draft and the tag (`gh release delete vX.Y.Z --cleanup-tag`), which only a
  repository admin can do, through the tag ruleset's bypass. Then revert the
  `chore(main): release X.Y.Z` commit through a pull request. That puts the
  manifest and `docs/changelog.md` back on the last published version, so
  release-please proposes X.Y.Z again, with every change since that release.
  Without the revert, it finds no release for the manifest's version, and
  proposes the next one with every change since the first commit.
- **The release pull request's checks wait for approval:** without a working
  `RELEASE_PLEASE_TOKEN` secret, as when it expired, release-please opens the
  pull request as `github-actions[bot]`, and GitHub holds the workflows it
  starts until a maintainer approves them. Approve them in the pull request's
  Checks tab, or approve each waiting run:

  ```bash
  gh run list --branch release-please--branches--main--components--claudes \
    --status action_required --json databaseId -q '.[].databaseId' |
    xargs -I{} gh api -X POST repos/uvw/claudes/actions/runs/{}/approve
  ```

  A CI run started by hand with `gh workflow run` does not count toward the
  required check.

## Repository settings

These live in GitHub, not in files, and are all in place. Apply them again when
moving or recreating the repository.

1. **Ruleset for `main`:** it requires a pull request, allows squash merges
   only, blocks force-pushes and deletion, and requires the "CI (all green)"
   check. It also requires CodeQL's results, and blocks a merge on an error
   alert or a security alert of high severity or higher.

   ```bash
   gh api -X POST repos/uvw/claudes/rulesets --input .github/rulesets/main.json
   ```

2. **Ruleset for release tags:** "release tags" covers `refs/tags/v*` and blocks
   moving, deleting, and force-pushing a tag. The repository admin role can
   bypass it, to repair a broken tag whose release is not published yet.

   ```bash
   gh api -X POST repos/uvw/claudes/rulesets --input .github/rulesets/tags.json
   ```

3. **Merge settings:** allow auto-merge and squash merging only, and delete
   branches after merge. A squash commit takes the pull request's title, the one
   CI checks, even when the pull request has a single commit.

   ```bash
   gh repo edit uvw/claudes --enable-auto-merge --enable-squash-merge \
     --enable-merge-commit=false --enable-rebase-merge=false \
     --delete-branch-on-merge
   gh api -X PATCH repos/uvw/claudes \
     -f squash_merge_commit_title=PR_TITLE -f squash_merge_commit_message=PR_BODY
   ```

4. **Actions may open pull requests,** so release-please can open its own, with
   workflow tokens read-only by default:

   ```bash
   gh api -X PUT repos/uvw/claudes/actions/permissions/workflow \
     -f default_workflow_permissions=read -F can_approve_pull_request_reviews=true
   ```

5. **The `release` environment:** its deployment policy admits only the `main`
   branch and `v*` tags. Every job that reads a release token runs in it: the
   release-please job and the changelog push in `release-please.yaml`, and
   publish, Homebrew, and winget in `release.yaml`. A workflow dispatched from
   any other branch cannot publish or read the tokens.

   ```bash
   gh api -X PUT repos/uvw/claudes/environments/release \
     -F 'deployment_branch_policy[protected_branches]=false' \
     -F 'deployment_branch_policy[custom_branch_policies]=true'
   gh api -X POST repos/uvw/claudes/environments/release/deployment-branch-policies \
     -f name=main -f type=branch
   gh api -X POST repos/uvw/claudes/environments/release/deployment-branch-policies \
     -f name='v*' -f type=tag
   ```

   `RELEASE_PLEASE_TOKEN`, `HOMEBREW_TAP_TOKEN`, and `WINGET_TOKEN` are secrets
   of this environment, not of the repository. `RELEASE_PLEASE_TOKEN` is a
   fine-grained token for this repository with contents and pull requests read
   and write. With it, release-please opens and updates the release pull request
   as you, so its checks start without approval. `hack/release-tokens.sh` sets
   all three, skipping any already set; name a secret to renew it, such as
   `hack/release-tokens.sh RELEASE_PLEASE_TOKEN`. Each token lasts a year.

6. **Releases:** immutable releases are on
   (`gh api -X PUT repos/uvw/claudes/immutable-releases`).
7. **Security:** private vulnerability reporting, Dependabot alerts and security
   updates, secret scanning, and push protection are on. Code scanning's default
   setup stays off, since `security.yaml` runs CodeQL itself.
