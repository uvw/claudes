# Contributing to claudes

## Design rules

1. One package per concern under `internal/`. `cli` holds the commands and
   depends on the others; the others never import `cli`.
2. The `claude` command line is reached only through the `claude.Agents`
   interface, and the terminal handoffs (`logs`, `attach`, passthrough, and
   `session resume`) only through `cli.App.Exec`, so every command can be tested
   with a fake.
3. Files under `~/.claude/jobs/` are read and written only in `internal/jobs`.
   The park file is owned by `internal/park`, `config.yaml` by
   `internal/config`, the directories claudes keeps its own files in by
   `internal/paths`, the transcripts under `~/.claude/projects` by
   `internal/transcripts`, the agent teams under `~/.claude/teams` and their
   task lists under `~/.claude/tasks` by `internal/teams`, the request files
   claudes and the claudes mod share by `internal/mod`, and installing the agent
   skill by `internal/install`. `internal/mcp` reads MCP configuration: the MCP
   keys of `~/.claude.json` and the settings files, `.mcp.json` files, plugins'
   MCP files, and `managed-mcp.json`. `internal/compat` reads the feature flags
   in `~/.claude.json`. `internal/mcp` writes one thing in it, a directory's
   `disabledMcpServers`, with a compare-and-swap, and nothing else writes it.
4. Side effects the tests replace (the clock, sleeping, Ctrl-C, stopping a
   process, and exec) are fields on `cli.App`.
5. Code that differs by platform goes in `_unix.go` and `_windows.go` files with
   build tags, never in runtime checks.
6. Keep to the README's [goals and non-goals](../README.md#goals-and-non-goals):
   claudes manages Claude Code sessions and does not orchestrate agents or
   support other coding agents.
7. Unknown commands pass through to `claude agents`. A new claudes command name
   or alias hides a `claude agents` argument of the same name, so check
   `claude agents --help` before adding one.

## Docs

The README's [Documentation](../README.md#documentation) table lists every doc.
Update a doc in the same change as the code it describes.

1. `README.md` is the front page: what claudes does, installing it, and a table
   of every command. Keep it short; detail goes in the user docs below it.
   `docs/install.md` covers installing, shell setup, and the agent skill,
   `docs/guide.md` how commands act and stay safe, and `docs/reference.md`
   output, exit codes, configuration, and files. The installers print a link to
   the README's `#shell-setup` section, so that heading stays in the README.
2. `docs/decisions.md` records each settled choice, why, and the alternatives.
   It is a living record: when a choice changes, rewrite its entry rather than
   adding one that replaces it, and keep the index table in step.
3. GitHub issues hold features, ideas, bugs, and other open work. Close one when
   it's done, or when a decision settles it.
4. `docs/handoffs.md` holds work one machine leaves for another, such as a
   Windows check made on a Mac, and the prompt that resumes the work.

The README opens with `docs/demo/demo.gif`. When a command it shows changes its
output, run `hack/demo.sh`, which needs Docker, to render it again from
`docs/demo/demo.tape`.

Say "directory", "dir" in column headers and placeholders such as `DIR`, or
"path" for a full path, in docs, help, messages, and code comments alike. Never
say "folder".

Prettier wraps Markdown at 80 columns. Run
`tools/node_modules/.bin/prettier --write` on a doc after editing it rather than
wrapping by hand.

## Checks

Run all of these before you commit, and keep them clean, with zero warnings:

```sh
gofmt -l .
go mod tidy -diff
go -C tools mod tidy -diff
go vet ./...
GOOS=windows go vet ./...
go test -race ./...
go tool -modfile=tools/go.mod staticcheck ./...
golangci-lint run ./...
go tool -modfile=tools/go.mod govulncheck ./...
go run hack/notices.go --check
goreleaser build --snapshot --clean
npm ci --prefix tools --ignore-scripts
tools/node_modules/.bin/prettier --check .
tools/node_modules/.bin/markdownlint-cli2 "**/*.md" "#dist" "#.claude/worktrees" "#docs/local" "#tools/node_modules"
```

Both `go mod tidy -diff` commands and `gofmt -l` must print nothing.

Every tool the checks run has a pinned version, and each pin is updated or
checked each week (decision 31):

1. staticcheck, govulncheck, and go-licenses are `tool` lines in `tools/go.mod`,
   a module of its own, so their dependencies stay out of the `go.mod` claudes
   ships, its notices, and the govulncheck scan. `go tool -modfile=tools/go.mod`
   builds each from that module and runs it on this one. PowerShell splits the
   flag at the dot, so quote it there: `"-modfile=tools/go.mod"`. Dependabot
   updates them.
2. Prettier and markdownlint-cli2 are in `tools/package.json`, and `npm ci`
   installs the versions `tools/package-lock.json` locks. Dependabot updates
   them.
3. goreleaser, golangci-lint, syft, zizmor, gitleaks, and komac stay pinned in
   the workflows. The weekly Tool pins workflow
   (`.github/workflows/tool-pins.yaml`) keeps an issue labeled `tool-pins` open
   while any of them is behind its latest release.

`goreleaser build` needs [goreleaser](https://goreleaser.com/install/) and
builds every release target. `hack/notices.go` builds go-licenses from
`tools/go.mod`, and fails on a license outside its allowlist. It lists each
module's license, repository, and full license text, but no versions, so a
version bump leaves the file unchanged. When a module comes or goes, or its
license changes, run it without `--check` to rewrite
`docs/third-party-notices.txt`.

[golangci-lint](https://golangci-lint.run/welcome/install/) needs v2.15.0 or
later, the first to read Go 1.27's packages. Run it with `GOOS=darwin` and
`GOOS=windows` too, since each platform's files build only for it, as CI does.
`.golangci.yaml` lists what it leaves out, and why.

CI runs the Go checks on Linux, macOS, and Windows, except `gofmt`, which skips
Windows. The coverage floor of 80% (`.github/workflows/hack/coverage.sh`),
govulncheck, the notices, and the Markdown checks run on Linux. Each platform
also builds and runs the release binary (`.github/workflows/hack/smoke.sh`) and
installs it with its install script. macOS does that inside its Go job, since a
macOS runner can take most of an hour to arrive, and runs on Intel, since
GitHub's Apple silicon runners are short on capacity. The "CI (all green)" job
is the one required check. Workflow steps longer than a few lines live in
`.github/workflows/hack/`, so they run the same way locally.

## Tests

Write tests alongside the code. Every behavior a user relies on gets a test that
fails without it, and CI keeps total coverage at 80% or more.

1. Command flows run in `internal/cli` tests against a fake `claude.Agents`, a
   temporary jobs directory, and a temporary park file. The fake accepts only an
   8-character job id for `stop`, `respawn`, and `rm`, as the real command line
   does, and models stops that land late and the host session dying.
2. `test/e2e` builds the real `claudes` and a fake `claude`
   (`test/e2e/fakeclaude`), puts only the fake on `PATH`, and runs the binary as
   a subprocess. It covers what the in-process fake cannot: process launch, the
   environment, exit codes, quoting, and the files claudes writes. It runs on
   Linux, macOS, and Windows, and `cmdshim_windows_test.go` checks npm's
   `claude.cmd` shim.
3. `internal/jobs` tests check `pins.json` byte for byte against the format
   agent view writes.
4. `internal/procs` tests stop stand-in processes named `claude`, never a real
   session.
5. Untested on purpose: the interactive handoff in `claude.Exec`. Check it by
   hand in a terminal, on each platform it changes.
6. The claudes mod in `plugins/claudes-mod` has TypeScript tests in `tests/`,
   which run against Claude Code's own engine. They need Claude Code, so CI does
   not run them. After changing the mod, run
   `claude plugin validate plugins/claudes-mod`, which reads the module the way
   a session loads it, and `claude plugin test plugins/claudes-mod`.

## Pull requests

Changes reach `main` through pull requests, which squash-merge. The pull request
title becomes the commit on `main`, so it follows the commit rules below; CI
checks it.

With write access to the repository:

1. Branch from `main`: `git switch -c fix/short-name`.
2. Open the pull request with `gh pr create --fill`, then turn on auto-merge
   with `gh pr merge --squash --auto`. It merges once "CI (all green)" passes.
3. Dependabot opens grouped pull requests weekly, titled `fix(deps): Bump ...`
   for the modules claudes ships and `ci(deps): Bump ...` for the tools and
   actions. The title check lets those through with their capital "Bump", from
   Dependabot only. Read the changelog of anything that changes behavior before
   you merge.

### From a fork

Without write access, work from a fork:

1. Fork `uvw/claudes`, clone your fork, and branch from `main`.
2. Run the [checks](#checks), then open the pull request against `uvw/claudes`
   with `gh pr create --fill`.
3. On your first pull request, CI waits until a maintainer approves its runs.
   Later ones run on every push. Runs from a fork get a read-only token and none
   of the repository's secrets.
4. A maintainer merges it once "CI (all green)" passes. You cannot turn on
   auto-merge yourself.

## Releases

Releases are automated from `main`; [releasing.md](releasing.md) is the full
runbook.

1. release-please keeps a pull request titled `chore(main): release X.Y.Z` with
   the next version and the `docs/changelog.md` entry.
2. Merging it tags `vX.Y.Z` and runs `.github/workflows/release.yaml`, which
   checks the tag is on `main`, runs CI on it, builds with goreleaser, smoke
   tests the `linux_amd64`, `darwin_arm64`, and `windows_amd64` archives,
   attests the build provenance, and publishes.
3. Once immutable releases are turned on (releasing.md, "Repository settings"),
   a published release cannot change. Fix a broken one by releasing the next
   version.

## Commits

Use Conventional Commits with a scope: `type(scope): subject`, with the subject
in lowercase imperative, for example `fix(cli): skip pinned sessions in gc`.
Types are `feat`, `fix`, `perf`, `docs`, `test`, `refactor`, `ci`, and `chore`.
`feat` and `fix` decide the next version and appear in the changelog. The scope
names the area: `cli`, `claude`, `compat`, `config`, `jobs`, `mcp`, `park`,
`paths`, `target`, `transcripts`, `teams`, `procs`, `install`, `safefile`,
`shell`, `selfupdate`, `style`, `trash`, and `version` for the packages under
`internal/`; `e2e` for `test/e2e`; `skill` for `skills/claudes`; `mod` for
`internal/mod` and `plugins/claudes-mod`; `hack` for the developer tools; `deps`
for Dependabot's updates; and `release` or `docs`. Leave the scope out only when
a commit spans several areas. Keep the body to one or two sentences about what
changed and why.
