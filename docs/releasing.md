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
   4. **Smoke:** each OS unpacks its own archive and runs
      `.github/workflows/hack/smoke.sh`, checking that `--version` reports the
      tag.
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
- **The release pull request has no CI run:** the `RELEASE_PLEASE_TOKEN` secret
  is missing or expired. A pull request opened with the default token starts no
  workflows, so its required check never runs.

## Repository settings

Apply these once, when the repository becomes public. They live in GitHub, not
in files.

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

3. **`RELEASE_PLEASE_TOKEN` secret:** a fine-grained token for this repository
   with contents and pull requests read and write.
   `gh secret set RELEASE_PLEASE_TOKEN`.
4. **Releases:** turn on immutable releases under Settings → General.
5. **Security:** turn on private vulnerability reporting, Dependabot alerts, and
   code scanning default setup off (`security.yaml` runs CodeQL itself).
