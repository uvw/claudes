# Releasing claudes

Releases are automated. Merging a pull request into `main` is the only manual
step.

## How a release happens

1. Pull requests squash-merge, so each PR title becomes one Conventional Commit
   on `main`. CI's "Pull request title" job enforces the format, and CI runs
   again when the title changes.
2. On every push to `main`, `.github/workflows/release-please.yaml` updates an
   open pull request titled `chore(main): release X.Y.Z`. It holds the next
   version and the `docs/changelog.md` entry, worked out from those commits:
   `feat` bumps the minor version and `fix` the patch while the version is below
   1.0.
3. Merging that pull request makes release-please create the `vX.Y.Z` tag and a
   draft release with the changelog as its notes. It then calls
   `.github/workflows/release.yaml` with the tag.
4. `release.yaml` runs, in order:
   1. **Eligibility:** the tag is `vX.Y.Z`, with no prerelease part, and is on
      `main`. Every later job builds the commit the tag named here, so moving
      the tag mid-run fails the build.
   2. **CI:** the whole of `ci.yaml` on the tag.
   3. **Build:** goreleaser builds every archive, `checksums.txt`, and one SBOM
      per archive, but publishes nothing.
   4. **Smoke:** all six archives, each on a runner of its own OS and
      architecture, are unpacked and run through
      `.github/workflows/hack/smoke.sh`, which checks that `--version` reports
      the tag, then installed with their install script.
   5. **Attest:** build provenance for every file in `checksums.txt`.
   6. **Publish:** uploads the assets to the draft and publishes it.

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

## When it fails

- **A workflow failed before publish:** the draft and the tag exist, and nothing
  is published. Fix the workflow on `main` through a pull request, then rerun
  with `gh workflow run release.yaml -f tag=vX.Y.Z`. A rerun takes the workflows
  from `main` but everything else from the tag, such as `go.mod`,
  `.goreleaser.yaml`, and the scripts under `.github/workflows/hack/`. The
  publish job uploads with `--clobber`, so it replaces any partial upload.
- **The fix is in the tagged code:** delete the draft and the tag
  (`gh release delete vX.Y.Z --cleanup-tag`) and merge the fix. release-please
  then proposes the next patch version, not the deleted one, and keeps the
  deleted version's changelog entry: remove it in the release pull request.
- **The release pull request's checks wait for approval:** without the
  `RELEASE_PLEASE_TOKEN` secret, release-please opens the pull request as
  `github-actions[bot]`, and GitHub holds the workflows it starts until a
  maintainer approves them. Approve them in the pull request's Checks tab, or
  approve each waiting run:

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
   check.

   ```bash
   gh api -X POST repos/uvw/claudes/rulesets --input .github/rulesets/main.json
   ```

2. **Merge settings:** allow auto-merge and squash merging only, and delete
   branches after merge. A squash commit takes the pull request's title, the one
   CI checks, even when the pull request has a single commit.

   ```bash
   gh repo edit uvw/claudes --enable-auto-merge --enable-squash-merge \
     --enable-merge-commit=false --enable-rebase-merge=false \
     --delete-branch-on-merge
   gh api -X PATCH repos/uvw/claudes \
     -f squash_merge_commit_title=PR_TITLE -f squash_merge_commit_message=PR_BODY
   ```

3. **Actions may open pull requests,** so release-please can open its own, with
   workflow tokens read-only by default:

   ```bash
   gh api -X PUT repos/uvw/claudes/actions/permissions/workflow \
     -f default_workflow_permissions=read -F can_approve_pull_request_reviews=true
   ```

   A `RELEASE_PLEASE_TOKEN` secret, a fine-grained token for this repository
   with contents and pull requests read and write, is optional. With it, the
   release pull request's checks start without approval.

4. **Releases:** immutable releases are on
   (`gh api -X PUT repos/uvw/claudes/immutable-releases`).
5. **Security:** private vulnerability reporting, Dependabot alerts and security
   updates, secret scanning, and push protection are on. Code scanning's default
   setup stays off, since `security.yaml` runs CodeQL itself.
