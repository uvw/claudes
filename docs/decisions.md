# Decisions

Each entry states a settled rule as it stands today, why, and what else was
weighed. When a rule changes, rewrite its entry rather than adding one that
replaces it. [contributing.md](contributing.md#docs) says how to keep this file
current.

## Index

| No.                                                                                 | Decision                                                                   |
| ----------------------------------------------------------------------------------- | -------------------------------------------------------------------------- |
| [1](#1-go-in-one-static-binary)                                                     | Go, in one static binary                                                   |
| [2](#2-mit-license)                                                                 | MIT license                                                                |
| [3](#3-anything-else-goes-to-claude-agents)                                         | Anything else goes to `claude agents`                                      |
| [4](#4-claude-commands-get-the-short-job-id)                                        | `claude` commands get the short job id                                     |
| [5](#5-pins-are-agent-views-pins-and-they-protect-sessions)                         | Pins are agent view's pins, and they protect sessions                      |
| [6](#6-safeguards-are-opt-in)                                                       | Safeguards are opt-in                                                      |
| [7](#7-interactive-sessions-are-opt-in-in-two-modes)                                | Interactive sessions are opt-in, in two modes                              |
| [8](#8-a-ping-is-a-stop-and-a-resume-with-a-prompt)                                 | A ping is a stop and a resume with a prompt                                |
| [9](#9-the-session-running-claudes-goes-last)                                       | The session running claudes goes last                                      |
| [10](#10-pause-saves-the-park-file-before-it-stops-anything)                        | Pause saves the park file before it stops anything                         |
| [11](#11-targets-match-exactly-then-by-prefix-then-by-part-of-a-name)               | Targets match exactly, then by prefix, then by part of a name              |
| [12](#12-claudes-reads-claudejobs)                                                  | claudes reads `~/.claude/jobs`                                             |
| [13](#13-releases-are-automated-attested-and-immutable)                             | Releases are automated, attested, and immutable                            |
| [14](#14-every-claude-call-has-a-time-limit)                                        | Every `claude` call has a time limit                                       |
| [15](#15-windows-kills-an-interactive-sessions-process-tree)                        | Windows kills an interactive session's process tree                        |
| [16](#16-resume-commands-use-the-platforms-shell)                                   | Resume commands use the platform's shell                                   |
| [17](#17-changes-reach-main-through-pull-requests)                                  | Changes reach `main` through pull requests                                 |
| [18](#18-settings-live-in-one-yaml-file-in-the-platforms-directory)                 | Settings live in one YAML file, in the platform's directory                |
| [19](#19-unfamiliar-claude-code-output-stops-writes)                                | Unfamiliar Claude Code output stops writes                                 |
| [20](#20-past-sessions-come-from-their-transcripts)                                 | Past sessions come from their transcripts                                  |
| [21](#21-claudes-cd-is-a-shell-function)                                            | `claudes cd` is a shell function                                           |
| [22](#22-few-dependencies-and-our-own-plumbing)                                     | Few dependencies, and our own plumbing                                     |
| [23](#23-one-claudes-changes-sessions-at-a-time-and-exit-codes-say-why-one-stopped) | One claudes changes sessions at a time, and exit codes say why one stopped |
| [24](#24-colors-come-from-lipgloss-and-only-reach-a-terminal)                       | Colors come from lipgloss, and only reach a terminal                       |
| [25](#25-claudes-reads-agent-teams-from-their-files-and-guards-their-leads)         | claudes reads agent teams from their files, and guards their leads         |
| [26](#26-every-session-listing-measures-disk-use)                                   | Every session listing measures disk use                                    |
| [27](#27-session-gc-moves-to-the-trash-and-never-touches-a-session-in-use)          | session gc moves to the trash, and never touches a session in use          |
| [28](#28-session-triage-sorts-and-the-agent-judges)                                 | session triage sorts, and the agent judges                                 |
| [29](#29-claudes-is-a-tool-for-claude-code-and-nothing-else)                        | claudes is a tool for Claude Code, and nothing else                        |
| [30](#30-short-flags-mean-one-thing-and-safeguards-stay-long)                       | Short flags mean one thing, and safeguards stay long                       |
| [31](#31-every-tool-pin-is-updated-or-checked-each-week)                            | Every tool pin is updated or checked each week                             |
| [33](#33-windows-replaces-a-file-with-posix-rename-semantics-first)                 | Windows replaces a file with POSIX rename semantics first                  |

## 1. Go, in one static binary

claudes is written in Go, with cobra for commands and completion, and ships as
one static binary per platform.

Why: a single file installs with `go install` or a release download, with no
interpreter to keep in step. Go's standard library covers processes, JSON, and
atomic file writes, and cobra gives zsh completion for free.

Weighed: Python, which needs a recent `python3` on every machine, older than
macOS ships; and Rust, which builds a smaller binary but takes longer to change
for a tool this size.

## 2. MIT license

claudes is MIT licensed.

Why: it's the simplest permissive license, and `go install` from a public repo
needs one for anyone else to use it.

Weighed: dual MIT and Apache-2.0. Apache's patent grant matters little for a
wrapper script.

## 3. Anything else goes to `claude agents`

`claudes` with no arguments, or with a first argument that isn't a claudes
command, `completion`, help, or `--version`, runs `claude agents` in its place
with those arguments. claudes' own `-v` or `--verbose` before the command is
skipped when it decides. `claudes -- ARGS` runs `claude agents ARGS` whatever
ARGS is.

Why: claudes is used in place of `claude agents`, so agent view and its flags
must keep working through it. A new claudes command hides any `claude agents`
argument of the same name, and `--` is the way around that.

Weighed: a `claudes agents` subcommand, which is one more word for the most
common use.

## 4. `claude` commands get the short job id

`claude stop`, `respawn`, `rm`, `logs`, and `attach` get the 8-character job id
from the `id` field of `claude agents --json`, never the full session id.

Why: those commands answer "No job matching" for the full session id, so passing
it would make every restart fail.

Weighed: nothing. The command line accepts only the job id.

## 5. Pins are agent view's pins, and they protect sessions

`claudes pin` and `unpin` edit `~/.claude/jobs/pins.json`, the file Ctrl+T in
agent view writes. Bulk `restart`, `pause`, and `gc` skip pinned sessions; a
session named as a target is included even when pinned, and `--pinned` has the
bulk command take every pinned session too, for a reboot or an update that
should leave nothing out.

Why: one pin list means a session pinned in agent view is protected in claudes
too, with nothing to keep in sync.

Weighed: a separate claudes pin list, which would drift from agent view; and
naming every session to get past the pins, which `--pinned` replaces with one
flag.

## 6. Safeguards are opt-in

By default, `restart` and `pause` act at once on every running background
session that isn't pinned. Waiting for busy sessions (`--wait`), asking first
(`--confirm`), and showing the plan only (`--dry-run`) are flags.

Why: claudes is an emergency tool, used when Claude Code has to restart now.
Default prompts and waits slow down the case it exists for.

Weighed: waiting and confirming by default.

## 7. Interactive sessions are opt-in, in two modes

Bulk commands leave interactive sessions alone and print the command that
resumes each one. `--interactive` stops them, parks them, and prints their
resume commands; `--interactive-bg` stops them and resumes them in the
background. A parked interactive session stays parked until it is seen running,
so `claudes resume` prints its command until then, and `resume --forget` drops
it.

Why: an interactive session belongs to a terminal claudes can't reach, and
stopping it by surprise loses the user's place.

Weighed: restarting interactive sessions by default, which ends work in a
terminal the user may be typing in.

## 8. A ping is a stop and a resume with a prompt

`--ping` stops a mid-turn session and resumes it with
`claude --bg --resume <id> "<prompt>"`. Mid-turn means busy, waiting on a
question or permission prompt, or idle with a background shell task running.
Idle sessions restart with `claude respawn` and get no prompt. Before the stop,
the session is parked; claudes then waits for its process to go, resumes it, and
unparks it. A session whose directory is gone is not stopped. `--wait` waits
only for busy sessions.

Why: `respawn` on a mid-turn session cuts its work short, and the session comes
back idle, waiting for input. The prompt tells it to carry on. Resuming a
session that still runs starts a copy, and a failed resume must not lose the
session. A waiting session needs a person, and a shell task such as a dev server
may never end, so waiting for either would run out the timeout.

Weighed: pinging every session, which wakes idle ones that had nothing to do;
and treating only `busy` as mid-turn, which restarts sessions mid-question and
kills their shell tasks unpinged.

## 9. The session running claudes goes last

The session claudes runs in, found by `CLAUDE_CODE_SESSION_ID`, or by the job id
in `CLAUDE_JOB_DIR` when that is not set, is restarted, paused, or stopped after
all the others and after everything else is printed, and is never pinged. An
interactive host is parked and its command printed, not resumed.

Why: stopping that session ends the claudes command, so anything after it would
never happen, including the resume commands and failures the user needs.

Weighed: refusing to touch the host session, which leaves it on an old Claude
Code after a bulk restart.

## 10. Pause saves the park file before it stops anything

`pause` writes the sessions it will stop to `parked.json` in claudes' state
directory first, then stops them. A session whose stop failed is dropped only if
it is seen still running, since a stop can take effect and still report an
error. `resume` removes each session it brings back, and deletes the file when
it's empty. Every change goes through a lock on the file.

Why: a crash or a reboot halfway through a pause must not lose the list of
sessions to bring back.

Weighed: writing the file after the stops, which loses it on a crash.

## 11. Targets match exactly, then by prefix, then by part of a name

A target selects the session whose name, job id, session id, or short id equals
it. Failing that, it selects the one session whose name or id starts with it,
ignoring case; the start of an id needs at least four characters. Failing that,
it selects the one session whose name contains it. Commands that act on running
sessions match against stopped ones too, and refuse a target that names one. The
first step with any match decides, and more than one match there is an error
that lists the candidates. `--skip` matches the same way, and a skip that
matches nothing is an error.

`ls` and `where` only look, so they return every match instead. A word matches
part of a name or the start of an id, and a target with `*`, `?`, or `[` is a
pattern over whole names and ids.

Why: names are what people remember, and a unique fragment is enough to type. A
short job id prefix must keep working when some name happens to contain it. An
ambiguous fragment must never pick a session at random, and a mistyped skip must
not leave the session it meant to protect unprotected.

Weighed: exact names only, which is tedious for long names; and fuzzy matching,
which can pick the wrong session. For `ls` and `where`, the stepwise rule hides
sessions: a name that starts with the word hides another that only contains it.
Matching the middle of an id is out, and the start of one needs four characters,
because random hex matches short words by accident: "2" or "db" would otherwise
pick a session by its id. Matching only running sessions is out too: a stopped
session named "api" let "restart api" fall through to a running "api-gw".

## 12. claudes reads `~/.claude/jobs`

Pins come from files under `~/.claude/jobs/`, read and written only in
`internal/jobs`. The Claude Code version a session runs comes from the path of
its process's executable, where the native installer names each version
(`…/claude/versions/2.1.326`), and otherwise from the job's `cliVersion` there.
`session mv` reads a stopped job's saved options, `respawnFlags`, before it
removes the job.

Why: `claude agents --json` has none of these, and pins, `restart --outdated`,
and the new job `session resume --bg` starts after a move need them.
`cliVersion` alone is stale after a respawn: Claude Code keeps the old value
until the session takes a turn, so a freshly restarted idle session looked
outdated and `--outdated` would restart it again. An npm install runs as `node`,
whose path names no version, so it still relies on `cliVersion`.

Weighed: leaving out pins and versions. The Claude Code docs call the directory
not a stable interface, so a Claude Code update may need claudes to follow it.

## 13. Releases are automated, attested, and immutable

release-please proposes each release as a pull request, from the Conventional
Commits on `main`. Merging it tags the version, and the release workflow checks
the tag is on `main`, runs CI on it, builds with goreleaser, smoke tests the
`linux_amd64`, `darwin_arm64`, and `windows_amd64` archives, attests build
provenance, and then publishes. Each archive ships with an SBOM. Releases become
immutable once the repository setting in `docs/releasing.md` is on.

Why: the commits already carry the version bump and the changelog, so a human
only decides when. Publishing after attestation means a release is never out
without its provenance, and immutability means a published version never changes
under anyone.

Weighed: tagging by hand, which keeps release notes and versions manual; and
drafts published by hand, which adds a click but no check that the workflow does
not already make.

## 14. Every `claude` call has a time limit

Listing, `--version`, and `rm` get one minute. `stop`, `respawn`, and resume get
three minutes. claudes also stops waiting for output five seconds after `claude`
exits, and a second Ctrl-C ends claudes at once. The handoffs that give `claude`
the terminal, such as `attach`, have no limit: they last as long as the user
keeps them open.

Why: a hung call would otherwise hang claudes for good, between a stop and its
resume. A background session that `claude` starts can inherit its output and
never close it, and cancelling the call does not end that wait.

Weighed: no limits, which is how the hang happened; and one short limit for
every call, which would cut off a slow but healthy stop.

## 15. Windows kills an interactive session's process tree

On Windows, `stop` and `--interactive` kill the session's process and every
process it started, then wait for it to exit. On macOS and Linux it gets
SIGTERM, then SIGKILL after ten seconds, and so does each process it started
that outlives it, found from a process list taken before the first signal.
Either way, claudes first checks that the process is Claude Code, and that it
started no later than the session's `startedAt`, with two seconds of slack, and
before the sessions were listed. It signals a child only while its start time
still matches. Claude Code is the native binary or node running the npm package,
which claudes tells from other node programs by the command line. Windows reads
that command line through `NtQueryInformationProcess`, which needs Windows 8.1
or later.

Why: Windows has no SIGTERM for console programs. Stopping only the session's
own process orphaned its shells and MCP servers. A reused PID must never be
killed. A session's process always starts first: measured, an interactive
session records `startedAt` about two seconds after its process starts, and a
background session runs in a spare process started ahead of time, minutes or
hours before. So only a process that started after the session is refused, and
the gap is never required to be small.

Weighed for macOS and Linux: signalling the process group, which misses a child
that started a group of its own.

Weighed for Windows: reading the command line from the process's memory, which
needs more access than claudes asks for, and asking WMI, which is slow to start.

Weighed: sending Ctrl-Break first and killing after a grace period, which is
untested against Claude Code and stays open in issue #9.

## 16. Resume commands use the platform's shell

The printed resume commands are POSIX shell commands on macOS and Linux, and
PowerShell commands on Windows. Each session's name prints on its own line,
without control characters.

Why: Windows PowerShell 5.1 rejects `&&`, and cmd cannot change directories and
quote safely in one line. PowerShell reads curly single quotes as quotes, so a
directory name could otherwise end the string and add a command.

Weighed: printing both forms, which doubles the output for every session.

## 17. Changes reach `main` through pull requests

A ruleset on `main` requires a pull request and the "CI (all green)" check, and
allows squash merges only. Auto-merge lands a pull request once it is green.

Why: the checks only protect `main` if they run before a change lands. Squash
merging makes each pull request title one Conventional Commit, which is what
release-please reads.

Weighed: committing to `main` with CI afterwards, which finds a break only once
it has landed; and merge commits, which put every branch commit into the
changelog.

## 18. Settings live in one YAML file, in the platform's directory

`config.yaml` holds defaults for flags: the ping prompt, the `--wait` timeout,
and the `-o` format. Its directory follows XDG on Linux and macOS and the
Windows known directories on Windows, and an absolute `XDG_` variable wins
everywhere. `internal/paths` names the directories, and `internal/config` reads
and writes the file. An unknown key or a bad value stops every command but those
that help fix it: `config`, `doctor`, `init`, `completion`, `skill`,
`--version`, and help.

Why: YAML is short to write by hand and reads the same on every platform. A typo
that claudes ignored would leave a setting silently off, which is worse for an
emergency tool than refusing to run.

Weighed: TOML, which needs a second parser for little gain; environment
variables alone, which do not persist across shells; and ignoring unknown keys,
which hides typos.

## 19. Unfamiliar Claude Code output stops writes

Every command that changes sessions or pins checks what it read first. A session
kind, status, job id, or session id outside the values claudes knows, or a
`pins.json` entry that is not a job id, stops the command before it acts.
`--force` overrides the stop. A Claude Code version outside `compat.Tested` only
warns. `claudes doctor` runs every check, and a weekly workflow runs
`claudes doctor --cli-only --strict` against the newest Claude Code and opens an
issue when it fails.

Why: claudes decides whom to ping and how to stop a session from these values.
An unknown status read as idle would restart a session mid-turn without a ping.
A version number alone cannot tell whether a release changed them, so the checks
look at the data itself, and the version only prompts a look.

Weighed: refusing every untested version, which blocks claudes after each Claude
Code update, almost always for nothing; warning without stopping, which lets a
changed status cut work short; and checking only in `claudes doctor`, which
nobody runs before a bulk restart.

## 20. Past sessions come from their transcripts

`claudes session` reads the transcripts under `~/.claude/projects`, since
`claude agents --json` knows only running and finished background sessions. It
reads the first 64 KB and the last 256 KB of each file, because Claude Code
writes the title and the last prompt again every few turns. It reads the whole
file only when neither end names the session's directory, or to count what a
session did: `session show` reads its one transcript, and `session gc --trivial`
reads every transcript up to 2 MB. It never changes a transcript. `session mv`
copies one, keeps the original, and prints the command that removes it. For a
stopped background session, it also removes the job with `claude rm` and keeps
the job's saved options in `moves.json`. `session resume --bg` passes them to
the new job, and only when no job exists. A name picks the newest session with
it, since sessions reuse names over time.

Why: reading both ends lists 50 sessions in about 12 ms, so no index is needed
to list them. Counting needs every line, and a session with more than 2 MB in it
is not trivial anyway: reading the 170 transcripts under 2 MB here adds about a
second, so that needs no index either. Copying instead of moving keeps the
session resumable from its old directory if the copy goes wrong, and a running
session keeps writing to its old transcript anyway. `claude --bg --resume` wakes
an existing job in the directory it started in, wherever it runs from. A live
check removed the job and ran the same command from the new directory. It
resumed the session itself under the same id, continued the copy's transcript,
and saved the options passed to the new job. Passing options to a job that still
exists starts a copy instead.

Weighed: reading each transcript in full, which reads hundreds of megabytes to
list sessions; moving the transcript, which loses the session if the new
directory is wrong; rewriting the directory recorded inside the transcript,
which `claude --resume` does not need; and keeping the job, which left
`session resume --bg` refusing the session until `claudes gc` removed it and its
options with it.

## 21. `claudes cd` is a shell function

`claudes init bash|zsh|fish|pwsh` prints a function named `claudes` that handles
`claudes cd NAME` in the shell and passes every other command to the program.
Without it, `claudes cd` explains how to load it.

Why: a program cannot change its parent shell's directory. zoxide and similar
tools work the same way, and one line in a profile is the usual cost.

Weighed: a subshell opened in the directory, which leaves the user one shell
deeper each time; and printing the directory only, which `claudes where` already
does.

## 22. Few dependencies, and our own plumbing

claudes depends on cobra, `go.yaml.in/yaml/v4`, `golang.org/x/sys`, and Charm's
lipgloss, `colorprofile`, and `x/term` for colors (decision 24), with
`google/go-cmp` in tests only. Atomic writes, the lock directory, platform
directories, shell quoting, and process checks stay in `internal/`.

Why: each of those must match what Claude Code does byte for byte, such as the
`pins.json.lock` directory proper-lockfile makes, or is a few dozen lines. The
packages that cover them either behave differently, such as a lock file in place
of a lock directory, or are archived. yaml v4 runs with v3's layout and no line
folding, so `-o yaml` output did not change with it.

Weighed: `google/renameio` and `natefinch/atomic` for writes, `gofrs/flock` for
locks, `adrg/xdg` for directories, `kballard/go-shellquote` for quoting, and
`hashicorp/go-version`, whose MPL license is outside the allowed ones.
`modernc.org/sqlite` and `sahilm/fuzzy` wait for session search.

## 23. One claudes changes sessions at a time, and exit codes say why one stopped

Commands that stop, start, remove, or pin sessions take a lock directory in
claudes' state directory for the whole run. A second run fails at once with
exit 5. `--dry-run` and the commands that only look take no lock. The lock
refreshes while it is held and goes stale 30 seconds after a claudes that died,
and claudes releases it before it stops its own session. Exit codes tell a
script what happened: 2 for a target that matched no session or several, 3 for
some sessions failed, 4 for a refused check, and 5 for the lock.

Why: two restarts at once could stop the same session twice, and a resume racing
a pause could start a copy. claudes is an emergency tool, so failing at once
beats waiting quietly behind another run. An agent that runs claudes needs to
tell "try another name" from "nothing changed" without parsing English.

Weighed: a lock per session, which still lets two runs plan against each other's
half-done work; waiting for the lock, which hides a stuck run; and one failure
code, which is what claudes had.

## 24. Colors come from lipgloss, and only reach a terminal

claudes styles text with Charm's lipgloss v2 (`charm.land/lipgloss/v2`) and
detects what a terminal shows with its `colorprofile`, and whether a stream is a
terminal with `x/term`. `internal/style` owns both: it decides per stream,
stdout and stderr apart, and lines tables up by what the terminal shows rather
than by bytes. `NO_COLOR`, set to any value but empty, and `CLICOLOR=0` turn
colors off, and `CLICOLOR=1` turns them on for a pipe. `CLICOLOR_FORCE` and
`TTY_FORCE`, which `colorprofile` honors on its own, do not. JSON, YAML,
completions, `init`, and `skill` stay plain. Colors use the 16 basic ANSI
colors, so they follow the terminal's theme.

Why: a terminal UI is planned (uvw/claudes#18), and Bubble Tea builds on
lipgloss, so the styles and detection carry over to it. `colorprofile` knows
Windows consoles, `TERM`, and tmux, and lipgloss turns on the virtual terminal
mode older Windows consoles need. Plain output for pipes keeps
`cd "$(claudes where NAME)"`, `grep`, and agents working. Every module it adds
is MIT, BSD, or Apache licensed.

Weighed: `fatih/color`, which has no layout or TUI to grow into;
`muesli/termenv` and lipgloss v1, which v2 replaces; `pterm`, which brings its
own widgets and look; and our own escape codes, which would mean our own
terminal detection. `text/tabwriter` counts escape bytes as width, so colored
tables could not keep using it.

## 25. claudes reads agent teams from their files, and guards their leads

claudes reads Claude Code's agent teams from
`~/.claude/teams/<team>/config.json` and the task lists under `~/.claude/tasks`,
both documented, and finds a teammate's transcript beside its lead's, under
`subagents/`. A lead's team has teammates at work while its config lists members
besides the lead. Bulk `restart` and `pause` skip such a lead, named ones go
ahead with a warning that names the teammates, and `restart --ping` prompts the
lead, so it can start them again. `gc --teams` removes team directories and task
lists of sessions that no longer run and have not changed for `--older-than`.

Why: stopping a lead stops its in-process teammates, which Claude Code cannot
resume, and `claude agents` lists only the lead, so nothing else shows the loss
coming. A live test showed it: a restarted lead's two teammates died mid-task,
and the pinged lead started new ones from scratch. Claude Code keeps a team
directory for every session once teams are on, and leaves them behind.

Weighed: refusing to restart a lead at all, which blocks the emergency use;
reading the undocumented `inFlight` field of a job's `state.json`, which the
team config makes unnecessary; and leaving team directories to Claude Code,
which kept them for weeks here.

## 26. Every session listing measures disk use

`claudes session` measures each session's disk use whenever it lists sessions:
the transcript, its sidecar directory of subagent transcripts and tool results,
the older copies `session mv` left, and Claude Code's backups of the files it
edited under `~/.claude/file-history`. Sizes are bytes of file content, links
are never followed, and `session du` (or `df`) adds them up by project
directory. Leftovers, the sidecar and file-history directories whose transcript
is gone, count only when nothing narrows the sessions.

Why: `session ls` already reads every transcript, and measuring the rest adds
about 50 ms to 0.5 s for 436 sessions here, so a SIZE column costs little and is
always there when the disk fills. File content is the same on every platform,
where allocated blocks are not.

Weighed: a `session ls --size` flag, which hides the column until someone thinks
to ask; and only `session du`, which totals directories but cannot say which
session is big. The command is `du`, since it measures files, not free space;
`df` is there for those who reach for it.

## 27. session gc moves to the trash, and never touches a session in use

`claudes session gc` removes a past session with everything Claude Code keeps
for it, and moves it to the system trash: the macOS Trash, the Windows Recycle
Bin, or the freedesktop.org trash. `--purge` deletes for good. Without flags it
removes empty sessions, older copies `session mv` left, and leftover
directories; `--trivial`, `--gone`, and `--older-than` are opt-in. A trivial
session changed no file, made no tool call, was not named with `/rename` or
started by an agent team, and either only ran slash commands or got one prompt.
`--sidecars` keeps each session and trims its sidecar directory: the command
outputs saved under `tool-results`, and with `--older-than` the subagent
transcripts of sessions idle that long, removing no whole session. It never
removes a running session, one agent view still lists, a parked one, or the
session running claudes, and without targets nothing that changed in the last
day. It lists what runs again just before removing, skips a transcript that
changed since the plan, and removes only paths under `~/.claude` whose names
carry the session's id.

Why: a transcript is the only record of a session's work, so a mistaken removal
must be recoverable, unlike `claudes gc`, which only drops a job from agent
view's list. Empty sessions, superseded copies, and orphaned directories hold
nothing anyone can resume, so they are safe to pick by default. A missing
directory can be a disk that is not attached, and age says nothing about value,
so those wait for a flag. So does `--trivial`: a one-prompt answer can still be
worth keeping, though one with no tool call holds only what the model already
knew. Which of the one-prompt sessions with tool calls to keep takes reading the
prompt, so claudes leaves that to the user or their agent. A test resumed
sessions with each part of the sidecar moved away: both resumed with their
context, since the transcript keeps a preview of each saved output and each
subagent's answer. Without `tool-results`, reading a saved output again fails;
without `subagents`, continuing a past subagent fails. Subagents held 1.3 GB
against 227 MB of saved outputs here, so they go too, but only for sessions old
enough that nobody continues their subagents. A session that changed in the last
day may be starting, before its transcript says what it is.

Weighed: deleting outright with `--dry-run` to preview, as `gc` does, which
cannot undo a mistake; previewing by default with `--yes` to act, which goes
against safeguards being opt-in (decision 6); and claudes' own trash directory,
which the system's tools cannot list or restore from.

## 28. session triage sorts, and the agent judges

`claudes session triage` sorts past sessions into remove, review, and keep, and
changes nothing. claudes decides remove and keep from facts it can count:
prompts, commands, tool calls, edits, file history, and where the name came
from. The review group, one-prompt sessions that changed no file and made at
most 20 tool calls, goes to whoever runs it: `-o json` gives each one's prompts,
and the agent skill tells an agent how to judge them and to ask before removing
any.

Why: the same shape fits a commit message written once and a pull request review
whose findings live nowhere else, and only reading the prompt tells them apart.
An agent the user already runs can read it, so claudes stays offline and
deterministic and sends no transcript text anywhere new. Above 20 tool calls, a
one-prompt session did enough work to keep.

Weighed: `session triage --agent`, which would run `claude -p` with a schema to
fill in the verdicts, at the cost of a model call, a time limit (decision 14),
and transcript text in a new process; and heuristics on the prompt's words,
which would remove the review that looked like a lookup.

## 29. claudes is a tool for Claude Code, and nothing else

claudes manages Claude Code's sessions from the command line, one job per
command. It does not orchestrate agents: it never plans work, splits it across
sessions, or decides what a session does next. It supports Claude Code only, not
other coding agents such as Copilot, Codex, or Pi. A feature that needs either
goes in another tool.

Why: claudes reads interfaces Claude Code does not promise to keep (decision
19), and every write depends on checks made for exactly that output. Each other
agent would need its own readers, checks, and fakes in the tests, which would
make claudes many tools in one. Orchestration is a product of its own, and
Claude Code already has agent teams and agent view for it. Staying small keeps
claudes safe to run against live work, which is the job it exists for.

Weighed: an adapter layer that would let claudes drive other agents' sessions;
and scheduling or chaining sessions, such as starting one when another finishes,
which is orchestration under another name.

## 30. Short flags mean one thing, and safeguards stay long

A flag people type often gets a one-letter form, and each letter means the same
thing on every command: `-n` is always a dry run, `-u` only sessions on an older
Claude Code, `-i` a directory to look in, `-l` how recent (the last 7 days),
`-b` the background, and `-r` resume. `-t` is how many to show: the last events
in `logs`, and the biggest sessions in `session du`. A flag that overrides a
safeguard never gets one: `--force`, `--purge`, `--pinned`, `--interactive`,
`--interactive-bg`, `--rm`, and `--forget`. A test walks every command and
enforces both rules.

Why: claudes is typed in a hurry, after an update or before a reboot, so the
common flags should be short. A letter that changes meaning between commands
turns muscle memory into a mistake. An override is the one flag whose typing
should take a moment, and `-f` already means follow.

Weighed: a short form for every flag, which runs out of letters and spends them
on flags almost nobody types; `-f` for `--force`, as many tools do, which
clashes with `logs -f`; and a default for bare `--top`, which would make
`--top 5` read the 5 as a session.

## 31. Every tool pin is updated or checked each week

The Go tools the checks run (staticcheck, govulncheck, and go-licenses) are
`tool` lines in `tools/go.mod`, a module of their own, run with
`go tool -modfile=tools/go.mod`. Prettier and markdownlint-cli2 are exact
versions in `tools/package.json`, with a committed lockfile. Dependabot updates
both each week. The tools the workflows hand to an action or download themselves
(goreleaser, golangci-lint, syft, zizmor, gitleaks, and komac) stay pinned in
the workflows and their scripts. The weekly Tool pins workflow keeps one issue
open that lists each one behind its latest release, and closes it when none are.

Why: a pin nobody updates goes stale without a word, and a stale scanner misses
what a newer one finds. A module of their own keeps the tools' dependencies out
of the `go.mod` claudes ships, its third-party notices, and the govulncheck scan
of the code users run. The workflow reads each version from the file that pins
it, so every version is written in one place.

Weighed: `tool` lines in the root `go.mod`, which would add the tools'
dependencies to the module claudes ships; `go run pkg@version`, which Dependabot
cannot see; and one file listing every pin, which each workflow would have to
read and keep in step with.

## 33. Windows replaces a file with POSIX rename semantics first

On Windows, an atomic write renames its temp file over the target with
`FILE_RENAME_FLAG_POSIX_SEMANTICS`. That replaces the target while other
processes have it open, as long as each opened it with delete sharing. Node and
Bun, which run Claude Code, share delete. Go's `os.Open`, which claudes reads
with, does not. When the POSIX rename fails for any reason, claudes falls back
to `os.Rename`, which is `MoveFileEx` and cannot replace an open file. Either
way, a target that stays held gets retries for a little over a second.

Why: agent view reads `pins.json` while claudes writes it, and a plain rename
waits until agent view closes the file. POSIX semantics need Windows 10 1607 or
later and a file system that has them, such as NTFS. Older builds, FAT, and
exFAT answer with errors such as invalid parameter, not supported, or invalid
function, and network shares may too. Falling back on any error covers a code
that list misses. Readers still see the old file or the new one: the rename is
one step, and a reader that has the old file open keeps reading it.

Weighed: POSIX semantics alone, which fail where the file system lacks them;
`FILE_RENAME_FLAG_IGNORE_READONLY_ATTRIBUTE`, which would overwrite a file the
owner made read-only and needs Windows 10 1809; and Go's `os.Root.Rename`, which
makes the same two attempts but needs an `os.Root` opened for every write.
