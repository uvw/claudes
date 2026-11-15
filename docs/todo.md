# To do

Work that blocks the first release and can't be a GitHub issue, because it needs
the owner, live sessions, or repository settings. Features, ideas, and
everything else live in the
[GitHub issues](https://github.com/uvw/claudes/issues). Delete an item when it's
done.

## Before the first release

1. Run every command for real, with the owner, against live sessions.
2. Check whether agent view writes `pins.json` through its newer storage layer,
   which may not take `pins.json.lock`.
3. Add the `RELEASE_PLEASE_TOKEN` secret (`docs/releasing.md`, "Repository
   settings"). Without it, the release pull request gets no CI run of its own,
   so start one on its branch with
   `gh workflow run ci.yaml --ref release-please--branches--main--components--claudes`.
4. Bump the prettier pin in `.github/workflows/ci.yaml` and
   `docs/contributing.md` from 3.9.10 to 3.10.1 on or after 2026-11-19, once it
   has passed the seven-day cooldown Dependabot uses. 3.10.1 already passes on
   the repository.
