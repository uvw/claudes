# Contributing to claudes

## Design rules

1. One package per concern under `internal/`, named with one lowercase word,
   with a doc comment that states its concern. `cli` holds three kinds of file:
   `root.go`; `app.go` and the `app_<concern>.go` files, which hold what the
   commands share; and one file per command, named for its path, such as
   `session_mv.go`. Code that needs neither `App` nor cobra goes in the package
   of its concern, when that package is more than a helper for one command.
   `cli` imports the other packages, and none of them imports `cli`.
   `AGENTS.md`, "Package layout", lists the rules and the tests that check them.
2. The `claude` command line is reached only through the `claude.Agents`
   interface, and the terminal handoffs (`logs`, `attach`, passthrough, and
   `session resume`) only through `cli.App.Exec`, so every command can be tested
   with a fake.
3. Each file and directory claudes reads or writes belongs to one package, and
   only that package opens it; the others call it.

   | File or directory                                                                                                      | Owner                  |
   | ---------------------------------------------------------------------------------------------------------------------- | ---------------------- |
   | `~/.claude/jobs/`, `pins.json` and each job's `state.json` among them                                                  | `internal/jobs`        |
   | `~/.claude/projects/`, and each session's `file-history`, `session-env`, and `debug` log                               | `internal/transcripts` |
   | `~/.claude/teams/` and `~/.claude/tasks/`                                                                              | `internal/teams`       |
   | `~/.claude.json`, for its MCP keys and the feature flags Claude Code caches                                            | `internal/claude`      |
   | `~/.claude/settings.json`, and a directory's `.claude/settings.json` and `.claude/settings.local.json`                 | `internal/mcp`         |
   | `.mcp.json` files                                                                                                      | `internal/mcp`         |
   | `~/.claude/plugins/installed_plugins.json`, and each plugin's `.mcp.json`, `.claude-plugin/plugin.json`, and MCP files | `internal/mcp`         |
   | `managed-mcp.json`                                                                                                     | `internal/mcp`         |
   | `moves.json`, in claudes' state directory                                                                              | `internal/transcripts` |
   | `trashed.json`, in claudes' state directory, and the system trash                                                      | `internal/trash`       |
   | `run.lock`, in claudes' state directory                                                                                | `internal/files`       |
   | The park file                                                                                                          | `internal/park`        |
   | `config.yaml`                                                                                                          | `internal/config`      |
   | The directories claudes keeps its own files in                                                                         | `internal/paths`       |
   | The cached `claude --version`                                                                                          | `internal/compat`      |
   | The installed agent skill                                                                                              | `internal/cli`         |

   Nothing in claudes writes `~/.claude.json`, the settings files, `.mcp.json`
   files, plugin files, or `managed-mcp.json`.

4. Side effects the tests replace (the clock, sleeping, Ctrl-C, stopping a
   process, exec, and the environment) are fields on `cli.App`, and only
   `cli/app.go` wires the real ones. Each one's real code lives in the package
   of its concern, such as `procs.Terminate`, or in an `app_` file when only
   `cli` needs it, such as the signals in `app_signals.go`.
5. Code that differs by platform goes in `_unix.go` and `_windows.go` files with
   build tags, never in runtime checks.
6. Keep to the README's [goals and non-goals](../README.md#goals-and-non-goals):
   claudes manages Claude Code sessions and does not orchestrate agents or
   support other coding agents.
7. Unknown commands pass through to `claude agents`. A new claudes command name
   or alias hides a `claude agents` argument of the same name, so check
   `claude agents --help` before adding one.
8. Safeguards, such as `--wait`, `--confirm`, and `--dry-run`, are flags, never
   defaults: claudes is an emergency tool
   ([decision 6](decisions.md#6-safeguards-are-opt-in)).

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
   It holds only what matters for the tool's design, behavior, critical
   operations, and safety, and that the code and the other docs do not already
   say. How claudes renders a path or a table, which library it uses, and how CI
   and releases run are evident elsewhere and get no entry. Give each entry its
   why and the alternatives weighed, with any measurements, and link the guide
   or the reference for the rule itself. It is a living record: when a choice
   changes, rewrite its entry rather than adding one that replaces it, and keep
   the index table in step. Never reuse a number, since issues and pull requests
   cite them.
3. GitHub issues hold features, ideas, bugs, and other open work. Close one when
   it's done, or when a decision settles it.
4. `docs/handoffs.md` holds work one machine leaves for another, such as a
   Windows check made on a Mac, and the prompt that resumes the work.
5. `skills/claudes/SKILL.md` is the agent skill, and it ships in the binary:
   `claudes skill` prints it, and `npx skills add uvw/claudes` reads it from the
   repository. Update it in the same change as any command, flag, or safety rule
   it describes.

The README opens with `docs/demo/demo.gif`. When a command it shows changes its
output, run `hack/demo.sh`, which needs Docker, to render it again from
`docs/demo/demo.tape`. It runs against the made-up sessions and transcripts
`docs/demo/setup.sh` makes, so a new scene that needs more adds them there.

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
go mod verify
go vet ./...
GOOS=windows go vet ./...
go test -race "-coverprofile=coverage.out" ./...
bash .github/workflows/hack/coverage.sh 80 coverage.out
go tool "-modfile=tools/go.mod" staticcheck ./...
GOOS=windows "$(go tool "-modfile=tools/go.mod" -n staticcheck)" ./...
GOOS=linux golangci-lint run ./...
GOOS=darwin golangci-lint run ./...
GOOS=windows golangci-lint run ./...
go tool "-modfile=tools/go.mod" govulncheck ./...
go run hack/notices.go --check
goreleaser build --snapshot --clean
zizmor --offline .
gitleaks git --redact .
npm ci --prefix tools --ignore-scripts
tools/node_modules/.bin/prettier --check .
tools/node_modules/.bin/markdownlint-cli2
bash .github/workflows/hack/release-notes.sh test
```

Both `go mod tidy -diff` commands and `gofmt -l` must print nothing. The quotes
keep PowerShell from splitting a flag at its dot. PowerShell also has no
`GOOS=windows command` form: run `$env:GOOS = "windows"` first, then the
command, then `Remove-Item Env:GOOS`. The coverage line needs bash, which Git
for Windows provides. The Windows staticcheck line runs a staticcheck built for
this machine, since `GOOS=windows go tool` would build one for Windows.

Every tool the checks run has a pinned version. These pins are updated or
checked each week:

1. staticcheck, govulncheck, and go-licenses are `tool` lines in `tools/go.mod`,
   a module of its own, so their dependencies stay out of the `go.mod` claudes
   ships, its notices, and the govulncheck scan. `go tool -modfile=tools/go.mod`
   builds each from that module and runs it on this one. Dependabot updates
   them.
2. Prettier and markdownlint-cli2 are in `tools/package.json`, and `npm ci`
   installs the versions `tools/package-lock.json` locks. Dependabot updates
   them. The `overrides` entry there pins katex, which markdownlint-cli2 pulls
   in, to a release with a security fix. Remove it once markdownlint-cli2 pulls
   in a fixed katex on its own.
3. goreleaser, golangci-lint, syft, zizmor, gitleaks, and komac stay pinned in
   the workflows, each written once per workflow. The weekly Tool pins workflow
   (`.github/workflows/tool-pins.yaml`) keeps an issue labeled `tool-pins` open
   while any of them is behind its latest release, and opens one when the check
   itself fails. A pull request that changes `.github/` gets a dry run of it.

Node, the Go toolchain, and the demo's VHS image are raised by hand instead,
since no weekly check fits them:

1. Node runs at the major version in `tools/.nvmrc`, an LTS line, and setup-node
   takes its newest release. Raise it when that line leaves active LTS.
2. The Go toolchain is the `go` line in `go.mod`, which setup-go and
   `GOTOOLCHAIN=auto` both read. Raise it when a Go release comes out.
3. The demo's VHS image is pinned by tag and digest in `hack/demo.sh`. Raise it
   when you render the demo again, since no check runs it.

`goreleaser build` needs [goreleaser](https://goreleaser.com/install/) and
builds every release target. `hack/notices.go` builds go-licenses from
`tools/go.mod`, and fails on a license outside its allowlist. It lists each
module's license, repository, and full license text, but no versions, so a
version bump leaves the file unchanged. When a module comes or goes, or its
license changes, run it without `--check` to rewrite
`docs/third-party-notices.txt`.

[golangci-lint](https://golangci-lint.run/welcome/install/) needs v2.15.0 or
later, the first to read Go 1.27's packages. It runs once for each `GOOS`, as CI
does, since each platform's files build only for it. It runs the default linters
(errcheck, govet, ineffassign, and unused), plus copyloopvar, errorlint,
misspell with US spelling, nilerr, unparam, and usestdlibvars. `.golangci.yaml`
lists what it leaves out, and why, including staticcheck, which runs on its own.
[zizmor](https://docs.zizmor.sh/installation/) and
[gitleaks](https://github.com/gitleaks/gitleaks#installing) need installing at
the versions `.github/workflows/ci.yaml` pins. zizmor runs offline, as in CI, so
it needs no token; the weekly Security workflow runs its online audits.
`.markdownlint-cli2.yaml` lists the files markdownlint-cli2 checks and the paths
it skips.

CI runs the Go checks on Linux and Windows, each on Intel and Arm, and on macOS,
except `gofmt`, which skips Windows. Windows on Arm runs the tests without the
race detector, which Go lacks there. The coverage floor of 80%
(`.github/workflows/hack/coverage.sh`) and govulncheck run on Linux on Intel,
and the notices, the Markdown checks, zizmor, and gitleaks on Linux. Each
platform also builds and runs the release binary
(`.github/workflows/hack/smoke.sh`) and installs it with its install script.
macOS does that inside its Go job, on Intel; `.github/workflows/ci.yaml` says
why. The "CI (all green)" job is the one required check, and a finding from
zizmor or gitleaks fails it. The weekly Security workflow
(`.github/workflows/security.yaml`) runs CodeQL, and runs zizmor and govulncheck
again for what arrives between pushes. Workflow steps longer than a few lines
live in `.github/workflows/hack/`, so they run the same way locally.

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
5. `internal/compat` parses samples of the real `claude agents --json` and
   `state.json` in its `testdata`. `TestLiveClaude` reads the real ones, and
   runs only in CI with `CLAUDES_LIVE_CLAUDE` set, so a local `go test` never
   reaches your own Claude Code. The weekly Compatibility workflow sets it
   through `.github/workflows/hack/compat-live.sh`, which starts a background
   job and so runs only in CI.
6. Untested on purpose: the interactive handoff in `claude.Exec`. Check it by
   hand in a terminal, on each platform it changes.

## Pull requests

Changes reach `main` through pull requests, which squash-merge. The pull request
title becomes the commit on `main`, so it follows the commit rules below; CI
checks it.

With write access to the repository:

1. Branch from `main`: `git switch -c fix/short-name`.
2. Open the pull request with `gh pr create --fill`, then turn on auto-merge
   with `gh pr merge --squash --auto`. It merges once "CI (all green)" passes.
3. Dependabot opens grouped pull requests weekly, and
   [releasing.md](releasing.md#how-a-release-happens) says how their titles pass
   the check. Read the changelog of anything that changes behavior before you
   merge.

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

Releases are automated from `main`: release-please keeps a pull request titled
`chore(main): release X.Y.Z`, and merging it tags, builds, and publishes the
release. [releasing.md](releasing.md) is the runbook.

## Commits

Use Conventional Commits with a scope: `type(scope): subject`, with the subject
in lowercase imperative and no closing period, for example
`fix(cli): skip pinned sessions in gc`. Types are `feat`, `fix`, `perf`, `docs`,
`test`, `refactor`, `ci`, and `chore`. `feat` and `fix` decide the next version
and appear in the changelog. The scope names the area: the package's name for
code under `internal/`, such as `cli` or `transcripts`; `e2e` for `test/e2e`;
`skill` for `skills/claudes`; `hack` for the developer tools; `deps` for
Dependabot's updates; and `release` or `docs`. Leave the scope out only when a
commit spans several areas. Keep the body to one or two sentences about what
changed and why.
