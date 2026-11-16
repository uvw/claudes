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
   `internal/config`, the folders claudes keeps its own files in by
   `internal/paths`, the transcripts under `~/.claude/projects` by
   `internal/transcripts`, the agent teams under `~/.claude/teams` and their
   task lists under `~/.claude/tasks` by `internal/teams`, and installing the
   agent skill by `internal/install`.
4. Side effects the tests replace (the clock, sleeping, Ctrl-C, stopping a
   process, and exec) are fields on `cli.App`.
5. Code that differs by platform goes in `_unix.go` and `_windows.go` files with
   build tags, never in runtime checks.
6. Unknown commands pass through to `claude agents`. A new claudes command name
   or alias hides a `claude agents` argument of the same name, so check
   `claude agents --help` before adding one.

## Docs

The README's [Documentation](../README.md#documentation) table lists every doc.
Update a doc in the same change as the code it describes.

1. `README.md` describes the commands, flags, and files for users.
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

Prettier wraps Markdown at 80 columns. Run `npx prettier --write` on a doc after
editing it rather than wrapping by hand.

## Checks

Run all of these before you commit, and keep them clean, with zero warnings:

```sh
gofmt -l .
go mod tidy -diff
go vet ./...
GOOS=windows go vet ./...
go test -race ./...
go run honnef.co/go/tools/cmd/staticcheck@v0.8.2 ./...
golangci-lint run ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...
go run hack/notices.go --check
goreleaser build --snapshot --clean
npx --yes prettier@3.9.10 --check .
npx --yes markdownlint-cli2@0.24.1 "**/*.md" "#dist" "#.claude/worktrees" "#docs/local"
```

`gofmt -l` and `go mod tidy -diff` must print nothing. `goreleaser build` needs
[goreleaser](https://goreleaser.com/install/) and builds every release target.
`hack/notices.go` needs [go-licenses](https://github.com/google/go-licenses) on
`PATH`, or its path in `$GO_LICENSES`. It also fails on a license outside its
allowlist. After a dependency change, run it without `--check` to rewrite
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
   temporary jobs folder, and a temporary park file. The fake accepts only an
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

## Pull requests

Changes reach `main` through pull requests, which squash-merge. The pull request
title becomes the commit on `main`, so it follows the commit rules below; CI
checks it.

1. Branch from `main`: `git switch -c fix/short-name`.
2. Open the pull request with `gh pr create --fill`, then turn on auto-merge
   with `gh pr merge --squash --auto`. It merges once "CI (all green)" passes.
3. Dependabot opens grouped pull requests weekly. Read the changelog of anything
   that changes behavior before you merge.

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
names the area: `cli`, `claude`, `compat`, `config`, `jobs`, `park`, `paths`,
`target`, `transcripts`, `teams`, `procs`, `install`, `safefile`, `shell`,
`selfupdate`, `style`, `trash`, and `version` for the packages under
`internal/`; `e2e` for `test/e2e`; `skill` for `skills/claudes`; `hack` for the
developer tools; and `release` or `docs`. Leave the scope out only when a commit
spans several areas. Keep the body to one or two sentences about what changed
and why.
