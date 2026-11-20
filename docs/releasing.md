# Releasing claudes

Releases are automated. Merging a pull request into `main` is the only manual
step.

## How a release happens

1. Pull requests squash-merge, so each PR title becomes one Conventional Commit
   on `main`. CI's "Pull request title" job enforces the format, and CI runs
   again when the title changes. Dependabot's titles, such as
   `fix(deps): Bump charm.land/lipgloss/v2 from 2.0.6 to 2.1.0`, pass with their
   capital "Bump", since Dependabot has no setting to lower it, but only from a
   pull request Dependabot opened. A `fix(deps)` update lands in the changelog's
   "Bug fixes" as **deps:** and makes a patch release, since it changes the
   binary. A `ci(deps)` update makes none.
2. On every push to `main`, `.github/workflows/release-please.yaml` updates an
   open pull request titled `chore(main): release X.Y.Z`. It holds the next
   version and the `docs/changelog.md` entry, worked out from those commits:
   `feat` bumps the minor version and `fix` the patch while the version is below
   1.0. release-please writes the entry with long lines, so the same workflow
   then formats `docs/changelog.md` in that pull request with Prettier
   (`.github/workflows/hack/format-changelog.sh`), in a job with no token that
   can push. When `COPILOT_TOKEN` is set, another job writes the
   [release notes](#release-notes). A later job pushes both in one commit with
   `RELEASE_PLEASE_TOKEN` (`.github/workflows/hack/release-pr-push.sh`), so the
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
   6. **Publish:** uploads the assets to the draft, puts the release notes above
      the changelog list when there are any, checks the tag again, and publishes
      it.

Publishing comes last, so a release is never out without its provenance.
Releases are immutable once published, so a broken release is fixed by releasing
the next version, never by editing it.

## Release notes

Copilot CLI writes notes people can read above the changelog list: a TL;DR, what
the release is about, and what changed in how commands act. It runs on every
push to `main` while a release pull request is open, and needs no step per
release.

1. The `release-notes` job in `release-please.yaml` gathers the release pull
   request's changelog entry and the title and body of each pull request merged
   since the last release, with the workflow's read-only token.
2. Copilot CLI writes `docs/release-notes/vX.Y.Z.md` from them, guided by
   `.github/release-notes/prompt.md`. Edit that file to change how the notes
   read.
3. The job checks the file, formats it with Prettier, and lints it with
   markdownlint. The push job checks it again and commits it to the release pull
   request beside the formatted changelog.
4. At publish, `release.yaml` puts the notes at the top of the release, then an
   "All changes" heading and the list release-please wrote.

`.github/workflows/hack/release-notes.sh` does each step, and
`release-notes.sh test` runs them on the fixtures in
`.github/release-notes/testdata`, with no Copilot call. CI runs that test.

Pull request bodies can come from anyone, so the workflow treats them as
untrusted:

1. The prompt marks them as data, never instructions, between markers a body
   cannot fake.
2. The job that runs Copilot holds only `COPILOT_TOKEN` and a read-only
   `GITHUB_TOKEN`, and gathers everything before Copilot starts.
3. Copilot sees one tool, `create`, which may write only the notes, in an empty
   directory with no MCP servers and no shell.
4. The check refuses anything but one file of Markdown under 16 KiB, with no
   HTML, images, @-mentions, title heading, or links outside this repository. A
   refused file is never pushed.
5. The push job accepts only `docs/changelog.md` and the notes for the release
   pull request's version.

Notes are optional, and never hold up a release. Without the `COPILOT_TOKEN`
secret the job skips itself with a notice. A failed run does not fail the
workflow, and the formatted changelog still goes out. Without a notes file at
the tag, or with one that fails its check, the release keeps release-please's
list alone, as before.

`COPILOT_TOKEN` is a secret of the `release` environment: a fine-grained token
of the repository owner's account with the account permission "Copilot Requests"
and no repository access. `hack/release-tokens.sh COPILOT_TOKEN` sets it up.

The model is `claude-sonnet-5.5` unless the repository variable
`RELEASE_NOTES_MODEL` names another, so changing it needs no code change:

```bash
gh variable set RELEASE_NOTES_MODEL --body claude-sonnet-5.5 -R uvw/claudes
```

Copilot CLI is pinned in `.github/release-notes/package.json`, and Dependabot
updates it weekly. A run takes about 10 to 15 Copilot AI credits, with one run
on each merge to `main`. Copilot CLI makes no more model calls once a run has
used 30 credits (`--max-ai-credits`), the least it accepts. The job log shows
what a run used.

## Cutting a release

1. `gh pr list --search "release in:title"` finds the release pull request.
2. Read its `docs/changelog.md` diff. If a commit landed with the wrong type,
   fix it with `Release-As: X.Y.Z` in the body of an empty commit, or edit the
   pull request's changelog before merging.
3. Read its release notes, `docs/release-notes/vX.Y.Z.md`, when there are any.
   Edit them in the pull request, or delete the file to release with the list
   alone. Each merge to `main` writes them again, so edit them just before you
   merge.
4. Merge it once CI is green: `gh pr merge --squash --auto <number>`.
5. Watch the release: `gh run watch` on the newest "Release Please" run.

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
Each token's name ends in the day it was made, since GitHub refuses a name
another of your tokens has. Revoke the old token once the new secret is saved.

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

   `RELEASE_PLEASE_TOKEN`, `HOMEBREW_TAP_TOKEN`, `WINGET_TOKEN`, and
   `COPILOT_TOKEN` are secrets of this environment, not of the repository.
   `RELEASE_PLEASE_TOKEN` is a fine-grained token for this repository with
   contents and pull requests read and write. With it, release-please opens and
   updates the release pull request as you, so its checks start without
   approval. `hack/release-tokens.sh` sets all four, skipping any already set;
   name a secret to renew it, such as
   `hack/release-tokens.sh RELEASE_PLEASE_TOKEN`. Each token lasts a year.

6. **Releases:** immutable releases are on
   (`gh api -X PUT repos/uvw/claudes/immutable-releases`).
7. **Security:** private vulnerability reporting, Dependabot alerts and security
   updates, secret scanning, and push protection are on. Code scanning's default
   setup stays off, since `security.yaml` runs CodeQL itself.
