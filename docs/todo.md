# To do

Work that blocks the first release and can't be a GitHub issue, because it needs
the owner, live sessions, or repository settings. Features, ideas, and
everything else live in the
[GitHub issues](https://github.com/uvw/claudes/issues). Delete an item when it's
done.

## Before the first release

1. Run every command for real, with the owner, against live sessions.
2. Check against the real `claude` that `claude stop` returns only once the job
   is down. A live test against Claude Code 2.1.325 showed that
   `claude --bg --resume` keeps the session id, wakes a session with its saved
   options, and prints "started a copy" when given new flags or a running
   session.
3. Check whether agent view writes `pins.json` through its newer storage layer,
   which may not take `pins.json.lock`.
4. Apply the repository settings in `docs/releasing.md`: the ruleset, merge
   settings, the release-please token, immutable releases, and private
   vulnerability reporting.
5. Check that GitHub finds the lowercase `docs/security.md` and
   `docs/contributing.md`: `gh api repos/uvw/claudes/community/profile` once the
   repository is public. GitHub documents the contributing file's name as case
   insensitive, but says nothing about the security policy's.
6. Turn the CI, Security, Release Please, and Compatibility workflows back on
   with `gh workflow enable`. They are off until the repository is public,
   because Actions cannot run on it before then.
7. Bump the prettier pin in `.github/workflows/ci.yaml` and
   `docs/contributing.md` from 3.9.10 to 3.10.1 on or after 2026-11-19, once it
   has passed the seven-day cooldown Dependabot uses. 3.10.1 already passes on
   the repository.
