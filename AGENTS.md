# Instructions for coding agents

Read `docs/contributing.md` first. Its rules apply to you in full; this file
adds what an agent needs on top of them.

@docs/contributing.md

## Where to look first

| You need                                        | Open                                                      |
| ----------------------------------------------- | --------------------------------------------------------- |
| What a command does and its flags               | `docs/guide.md`, then `claudes <cmd> --help`              |
| Why something is the way it is                  | `docs/decisions.md`                                       |
| What's open                                     | GitHub issues, `docs/handoffs.md`                         |
| Where code goes, and the tests that check it    | "Package layout" below, `internal/layout`                 |
| A command's code                                | `internal/cli/<path>.go`, such as `session_mv.go`         |
| The command tree and passthrough                | `internal/cli/root.go`                                    |
| The `App` and the real side effects it wires    | `internal/cli/app.go`                                     |
| Printing, warnings, the log, and `writeTable`   | `internal/cli/app_output.go`                              |
| Plans, progress, the report, and the gate       | `internal/cli/app_report.go`, `internal/render/report.go` |
| Flags several commands share                    | `internal/cli/app_flags.go`                               |
| Shell completion                                | `internal/cli/app_complete.go`                            |
| Listing sessions, past sessions, and team leads | `internal/cli/app_sessions.go`                            |
| Which sessions a bulk command picks             | `internal/selection`, `internal/cli/app_selection.go`     |
| The session running claudes                     | `internal/selection/host.go`                              |
| Stopping, parking, and relaunching              | `internal/cli/app_relaunch.go`                            |
| Handing the terminal to `claude`                | `internal/cli/app_handoff.go`                             |
| The compatibility gate and installed version    | `internal/cli/app_compat.go`, `internal/compat`           |
| The run lock                                    | `internal/files/runlock.go`, `internal/cli/app_lock.go`   |
| The rows `ls` and `where` print                 | `internal/cli/app_listing.go`                             |
| Past sessions: gc, triage, du, show, and search | `internal/history`                                        |
| The signals that end a run                      | `internal/cli/app_signals.go`, one file per platform      |
| Error types and exit codes                      | `internal/cli/app_exit.go`                                |
| The calls to the `claude` command line          | `internal/claude/client.go`                               |
| Reading `~/.claude.json`                        | `internal/claude/config.go`                               |
| Agent view's pins and per-job versions          | `internal/jobs`                                           |
| The version a running session runs              | `internal/procs`, `internal/cli/app_compat.go`            |
| Comparing versions, and claudes' own            | `internal/version`                                        |
| The park file                                   | `internal/park`                                           |
| `config.yaml` and the editor                    | `internal/config`                                         |
| Reading and copying transcripts                 | `internal/transcripts`                                    |
| How `session search` matches and ranks          | `internal/transcripts/search.go`                          |
| Moving files to the system trash                | `internal/trash`, one file per platform                   |
| Agent teams, task lists, and `team ls` rows     | `internal/teams`                                          |
| MCP servers and profiles                        | `internal/mcp`                                            |
| Where claudes keeps its own files               | `internal/paths`, one file per platform                   |
| Checking directories: within, same, gone        | `internal/paths/dirs.go`                                  |
| Atomic writes and lock directories              | `internal/files`                                          |
| Colors, tables, cells, and `-o` formats         | `internal/render`                                         |
| Fitting tables to the terminal                  | `internal/render/fit.go`, `internal/cli/app_output.go`    |
| `claudes upgrade` and its downloads             | `internal/upgrade`                                        |
| How a typed name or id finds a session          | `internal/target`                                         |
| Resume and remove commands, and quoting         | `internal/shell`, one file per platform                   |
| Stopping an interactive session                 | `internal/procs`, one file per platform                   |
| The agent skill and `claudes skill`             | `skills/claudes/SKILL.md`, `internal/cli/skill.go`        |
| The compiled binary against a fake claude       | `test/e2e`, with the fake in `fakeclaude/`                |
| CI, security scans, and releases                | `.github/workflows/`, `docs/releasing.md`                 |
| The release notes' prompt, tests, and pin       | `.github/release-notes/`                                  |
| The checks' tools and their pins                | `tools/`, `.github/workflows/hack/tool-pins.sh`           |
| Scripts the workflows run                       | `.github/workflows/hack/`                                 |
| The install scripts                             | `install/install.sh`, `install/install.ps1`               |
| Developer tools, such as the notices file       | `hack/`                                                   |
| Homebrew, winget, and their tokens              | `docs/releasing.md`, "Package managers"                   |
| The README's demo GIF                           | `docs/demo/`, rendered by `hack/demo.sh`                  |

## Package layout

Tests in `internal/cli/root_test.go` and `internal/layout/layout_test.go` check
rules 1 and 2, and CI checks rule 5's coverage floor.

1. **`internal/cli` holds three kinds of file.** `root.go`; `app.go` and the
   `app_<concern>.go` files, which hold what the commands share; and one file
   per command, named for its path, such as `session_mv.go` for
   `claudes session mv`. An `app_` file built for one platform ends in `_unix`
   or `_windows`. Test files take the same names, and `harness_test.go` holds
   the harness they share. `TestEveryFileIsACommandAnAppFileOrRoot` checks the
   names, and `TestEachCommandLivesInItsFile` checks that each command runs from
   its own file.
2. **A package's name is one lowercase word, and its doc comment states its
   concern.** `TestPackageNamesAreOneWord` checks the name, and its `twoWords`
   list keeps old names such as `safefile` from coming back.
   `TestEveryPackageStatesItsConcern` checks the doc comment.
3. **Code that needs neither `App` nor cobra goes in a package of its concern**
   under `internal/`, when that package holds more than a helper for one
   command. A small helper stays in `cli`, in its command's file or an `app_`
   file.
4. **Side effects stay injectable as `App` fields.** The clock, sleeping,
   Ctrl-C, stopping a process, exec, and the environment go through a field on
   `App`, and only `app.go` wires the real ones.
5. **Tests sit with their code.** A function's unit tests move with it. CI
   counts coverage for each package by its own tests, so code moved without its
   tests pulls the total under the 80% floor
   (`.github/workflows/hack/coverage.sh`).

## The owner's sessions

The owner's Claude Code sessions and their claudes install are live work. On the
owner's machine, run only the commands that "Before you change anything" in
`skills/claudes/SKILL.md` lists as read-only. Every other command needs the
owner's yes first, even one the skill lets an agent run when its user asks: that
includes `upgrade`, `skill --install`, `config set`, `config edit`, and
`gc --teams`. Never edit `~/.claude/jobs/` or `~/.claude/projects/` by hand.

Tests drive the commands through the fake `claude.Agents` in
`internal/cli/harness_test.go` and a temporary jobs directory, never the real
`claude` binary. To try a command against real sessions, ask the owner first.

## Sharing the working tree

Several agents may work in this checkout at once. Changes land through pull
requests (docs/contributing.md, "Pull requests"), each from its own branch: the
ruleset on `main` refuses a direct push.

1. Commit only your own paths: `git add -- <paths>` and
   `git commit -m ... -- <paths>`. Never `git add -A`, stash, reset, or amend a
   commit that isn't yours.
2. If git reports `index.lock`, wait a few seconds and retry. Never delete the
   lock.
3. Re-read a shared file, such as `docs/decisions.md`, right before you edit it.
   Check the last decision number just before you add an entry.
4. Before you report work as done, run every command under "Checks" in
   `docs/contributing.md`. If a failure comes from another agent's uncommitted
   files, report it instead of fixing it.

## Writing on GitHub

Write issue and pull request bodies and comments with each paragraph and list
item on one line, however long. GitHub shows a line break inside a paragraph as
a break, so text wrapped at 80 columns shows up as ragged short lines. The
80-column wrap in `docs/contributing.md` applies only to the repository's
Markdown files, which Prettier formats. Pass the body with `--body-file` or a
heredoc, and never run it through Prettier.

## Gotchas

1. **`claude stop`, `respawn`, `rm`, `logs`, and `attach` take the short job
   id.** The full session UUID gets "No job matching". Use `Session.JobID`; the
   fake in the tests rejects anything else.
2. **Only background sessions have a job id.** Interactive sessions are stopped
   through `procs.Terminate` and resumed with `claude --resume <session id>`.
3. **`claude respawn` on a mid-turn session interrupts it, and it comes back
   idle.** Mid-turn is `busy`, `waiting`, or `shell` (`Session.MidTurn`), and
   `App.midTurn` adds a team lead with teammates at work. A ping is a stop, then
   `claude --bg --resume <id> "<prompt>"`, through `App.relaunch`, which parks
   the session first and waits for the stop to land. Resuming a session that
   still runs starts a copy. So does passing flags, such as `--model`, to
   `--bg --resume`: Claude Code already wakes a background session with the
   options it started with.
4. **`~/.claude/jobs/` and `claude agents --json` are not stable Claude Code
   interfaces.** `internal/compat` checks what claudes reads, and a write
   command refuses what it does not recognize. When the weekly Compatibility
   workflow opens an issue, run `claudes doctor`, update `kinds`, `statuses`, or
   `commands` in `internal/compat` as needed, refresh the scrubbed samples in
   `internal/compat/testdata` from the run's log, and raise `compat.Tested`.
5. **`pins.json` must stay byte-compatible with agent view.** It is a JSON array
   indented by two spaces with no trailing newline, written atomically under a
   `pins.json.lock` directory that goes stale after five seconds. Unpinning also
   deletes `jobs/<id>/order`.
6. **The session running claudes goes last and is never pinged.** Stopping it
   ends the command, so every report, resume command, and park write must come
   first. The tests model this: `harness.host` cuts all output after the host
   stops.
7. **`procs.Terminate` needs the session's start and the time the sessions were
   listed.** It refuses a process that started after either, since its PID was
   reused. Pass `Session.Started()`, and list through `App.listed`, which
   records the other.
8. **`claude` runs without the calling session's variables.** `execRunner` drops
   `CLAUDECODE`, `CLAUDE_JOB_DIR`, and the rest of `sessionVars`, so a session
   claudes starts is not taken for a child of the one claudes runs in. Add any
   new session-scoped variable there.
9. **Change the park file only through `park.File.Update`.** It holds the park
   file's lock, so two claudes runs cannot lose each other's entries.
10. **Every path a table shows goes through `render.DirCell`, every branch
    through `render.BranchCell`, and every table through `app.writeTable`.** A
    path or branch that skips them wraps every row, and
    `TestTablesPutPathsThroughDirCell` or
    `TestTablesPutBranchesThroughBranchCell` fails.
11. **A transcript record type claudes does not know keeps the session out of
    `session gc`'s empty and trivial kinds.** When Claude Code adds a type that
    holds no conversation, add it to `bookkeeping` in
    `internal/transcripts/transcripts.go`, or such sessions pile up.

## Maintaining this file

Update this file in the same change as any convention it describes: a new doc, a
moved file, a new trap, or a new rule for sharing the checkout. Keep each gotcha
to a bold rule and its consequence, and delete one that no longer holds.
