# Instructions for coding agents

Read `docs/contributing.md` first. Its rules apply to you in full; this file
adds what an agent needs on top of them.

@docs/contributing.md

## Where to look first

| You need                                     | Open                                                            |
| -------------------------------------------- | --------------------------------------------------------------- |
| What a command does and its flags            | `docs/guide.md`, then `claudes <cmd> --help`                    |
| Why something is the way it is               | `docs/decisions.md`                                             |
| What's open                                  | GitHub issues, `docs/handoffs.md`                               |
| Passthrough, wiring, and the host session    | `internal/cli/root.go`                                          |
| The run lock, and the signals that end a run | `internal/cli/root.go`, `internal/cli/signals_*.go`             |
| Error types and exit codes                   | `internal/cli/errors.go`                                        |
| What the commands share                      | `internal/cli/{plan,report,flags,output,cells,dirs,version}.go` |
| Restart, pause, and resume                   | `internal/cli/restart.go`, `pause.go`, `resume.go`              |
| Which sessions a bulk command picks          | `internal/cli/selection.go`                                     |
| Stopping, parking, and relaunching           | `internal/cli/relaunch.go`                                      |
| Ping, stop, logs, and attach                 | `internal/cli/{ping,stop,logs,attach}.go`                       |
| ls and where                                 | `internal/cli/ls.go`, `internal/cli/where.go`                   |
| gc                                           | `internal/cli/gc.go`                                            |
| The calls to the `claude` command line       | `internal/claude/client.go`                                     |
| Agent view's pins and per-job versions       | `internal/jobs/jobs.go`                                         |
| The version a running session runs           | `internal/procs`, `internal/cli/version.go`                     |
| The park file                                | `internal/park/park.go`                                         |
| `config.yaml` and `claudes config`           | `internal/config`, `internal/cli/config*.go`                    |
| Compatibility checks and `claudes doctor`    | `internal/compat`, `internal/cli/doctor.go`                     |
| `claudes doctor env`                         | `internal/cli/doctor_env.go`                                    |
| `claudes session`, `cd`, and `init`          | `internal/cli/session*.go`, `{cd,init}.go`                      |
| Reading and copying transcripts              | `internal/transcripts`                                          |
| How `session search` matches and ranks       | `internal/transcripts/search.go`                                |
| Disk use and removing past sessions          | `internal/cli/session_{du,gc,triage}.go`                        |
| Moving files to the system trash             | `internal/trash`, one file per platform                         |
| Agent teams and their task lists             | `internal/teams`, `internal/cli/team*.go`                       |
| MCP servers, profiles, and `claudes mcp`     | `internal/mcp`, `internal/cli/mcp*.go`                          |
| Where claudes keeps its own files            | `internal/paths`, one file per platform                         |
| Atomic writes and lock directories           | `internal/safefile`                                             |
| Colors and the text tables' layout           | `internal/style`                                                |
| Fitting tables to the terminal               | `internal/style`, `internal/cli/output.go`                      |
| CI, security scans, and releases             | `.github/workflows/`, `docs/releasing.md`                       |
| The checks' tools and their pins             | `tools/`, `.github/workflows/hack/tool-pins.sh`                 |
| `claudes upgrade` and its downloads          | `internal/cli/upgrade.go`, `internal/selfupdate`                |
| Scripts the workflows run                    | `.github/workflows/hack/`                                       |
| The install scripts                          | `install/install.sh`, `install/install.ps1`                     |
| Developer tools, such as the notices file    | `hack/`                                                         |
| Homebrew, winget, and their tokens           | `docs/releasing.md`, "Package managers"                         |
| The README's demo GIF                        | `docs/demo/`, rendered by `hack/demo.sh`                        |
| The agent skill and `claudes skill`          | `skills/claudes/SKILL.md`, `internal/install`                   |
| How a typed name or id finds a session       | `internal/target/target.go`                                     |
| Resume and remove commands, and quoting      | `internal/shell`, one file per platform                         |
| Stopping an interactive session              | `internal/procs`, one file per platform                         |
| The compiled binary against a fake claude    | `test/e2e`, with the fake in `fakeclaude/`                      |

Files in `internal/cli` take their command's path as a prefix, such as
`session_mv.go` for `claudes session mv` and `doctor_env.go` for
`claudes doctor env`. The parent's file holds what its subcommands share, and
small subcommands, such as `config get`, stay in it.
`TestEachCommandLivesInItsFile` checks each command's file, and its
`inParentsFile` lists the exceptions, each with why. Code that several commands
use goes in a file named for what it does, such as `cells.go` for table cells,
never in one command's file. Test files follow the same names, and `cli_test.go`
holds the harness they share.

## The owner's sessions

The owner's Claude Code sessions and their claudes install are live work. On the
owner's machine, run only the commands that "Before you change anything" in
`skills/claudes/SKILL.md` lists as read-only. Every other command needs the
owner's yes first, even one the skill lets an agent run when its user asks: that
includes `upgrade`, `skill --install`, `config set`, `config edit`, and
`gc --teams`. Never edit `~/.claude/jobs/` or `~/.claude/projects/` by hand.

Tests drive the commands through the fake `claude.Agents` in
`internal/cli/cli_test.go` and a temporary jobs directory, never the real
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
10. **Every path a table shows goes through `dirCell`, every branch through
    `branchCell`, and every table through `app.writeTable`.** A path or branch
    that skips them wraps every row, and `TestTablesPutPathsThroughDirCell` or
    `TestTablesPutBranchesThroughBranchCell` fails.
11. **A transcript record type claudes does not know keeps the session out of
    `session gc`'s empty and trivial kinds.** When Claude Code adds a type that
    holds no conversation, add it to `bookkeeping` in
    `internal/transcripts/transcripts.go`, or such sessions pile up.

## Maintaining this file

Update this file in the same change as any convention it describes: a new doc, a
moved file, a new trap, or a new rule for sharing the checkout. Keep each gotcha
to a bold rule and its consequence, and delete one that no longer holds.
