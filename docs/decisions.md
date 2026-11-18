# Decisions

Each entry records a settled choice about how claudes behaves or stays safe,
why, and what else was weighed. The rule itself lives in the guide, the
reference, or the code, and an entry links there rather than restating it. When
a choice changes, rewrite its entry rather than adding one that replaces it.
Numbers are never reused, since issues and pull requests cite them, so a removed
entry leaves a gap. [contributing.md](contributing.md#docs) says what belongs
here.

## Index

| No.                                                                                 | Decision                                                                   |
| ----------------------------------------------------------------------------------- | -------------------------------------------------------------------------- |
| [3](#3-anything-else-goes-to-claude-agents)                                         | Anything else goes to `claude agents`                                      |
| [5](#5-pins-are-agent-views-pins-and-they-protect-sessions)                         | Pins are agent view's pins, and they protect sessions                      |
| [6](#6-safeguards-are-opt-in)                                                       | Safeguards are opt-in                                                      |
| [7](#7-interactive-sessions-are-opt-in-in-two-modes)                                | Interactive sessions are opt-in, in two modes                              |
| [8](#8-a-ping-is-a-stop-and-a-resume-with-a-prompt)                                 | A ping is a stop and a resume with a prompt                                |
| [9](#9-the-session-running-claudes-goes-last)                                       | The session running claudes goes last                                      |
| [10](#10-pause-saves-the-park-file-before-it-stops-anything)                        | Pause saves the park file before it stops anything                         |
| [11](#11-targets-match-exactly-then-by-prefix-then-by-part-of-a-name)               | Targets match exactly, then by prefix, then by part of a name              |
| [12](#12-a-sessions-version-comes-from-its-executable)                              | A session's version comes from its executable                              |
| [14](#14-every-claude-call-has-a-time-limit)                                        | Every `claude` call has a time limit                                       |
| [15](#15-stopping-an-interactive-session-kills-its-process-tree)                    | Stopping an interactive session kills its process tree                     |
| [16](#16-resume-commands-use-the-platforms-shell)                                   | Resume commands use the platform's shell                                   |
| [18](#18-settings-live-in-one-yaml-file-and-a-typo-stops-claudes)                   | Settings live in one YAML file, and a typo stops claudes                   |
| [19](#19-unfamiliar-claude-code-output-stops-writes)                                | Unfamiliar Claude Code output stops writes                                 |
| [20](#20-past-sessions-come-from-their-transcripts)                                 | Past sessions come from their transcripts                                  |
| [22](#22-few-dependencies-and-our-own-plumbing)                                     | Few dependencies, and our own plumbing                                     |
| [23](#23-one-claudes-changes-sessions-at-a-time-and-exit-codes-say-why-one-stopped) | One claudes changes sessions at a time, and exit codes say why one stopped |
| [25](#25-claudes-reads-agent-teams-from-their-files-and-guards-their-leads)         | claudes reads agent teams from their files, and guards their leads         |
| [27](#27-session-gc-moves-to-the-trash-and-never-touches-a-session-in-use)          | session gc moves to the trash, and never touches a session in use          |
| [28](#28-session-triage-sorts-and-the-agent-judges)                                 | session triage sorts, and the agent judges                                 |
| [29](#29-claudes-is-a-tool-for-claude-code-and-nothing-else)                        | claudes is a tool for Claude Code, and nothing else                        |
| [30](#30-short-flags-mean-one-thing-and-safeguards-stay-long)                       | Short flags mean one thing, and safeguards stay long                       |
| [32](#32-session-search-scans-every-transcript-with-no-index)                       | session search scans every transcript, with no index                       |
| [33](#33-windows-replaces-a-file-with-posix-rename-semantics-first)                 | Windows replaces a file with POSIX rename semantics first                  |
| [34](#34-mcp-control-starts-read-only-and-never-prints-a-servers-secrets)           | MCP control starts read-only, and never prints a server's secrets          |
| [35](#35-session-search-masks-secrets-in-what-it-prints)                            | session search masks secrets in what it prints                             |

## 3. Anything else goes to `claude agents`

Anything that is not a claudes command runs `claude agents` with the same
arguments, and `claudes -- ARGS` always does ([Commands](guide.md#commands)).

Why: claudes is used in place of `claude agents`, so agent view and its flags
must keep working through it. A new claudes command hides any `claude agents`
argument of the same name, and `--` is the way around that.

Weighed: a `claudes agents` subcommand, which is one more word for the most
common use.

## 5. Pins are agent view's pins, and they protect sessions

`claudes pin` and `unpin` edit the `pins.json` that Ctrl+T in agent view writes,
and bulk `restart`, `pause`, and `gc` skip pinned sessions
([Safety](guide.md#safety)).

Why: one pin list means a session pinned in agent view is protected in claudes
too, with nothing to keep in sync. `--pinned` takes every pinned session in one
flag, for a reboot or an update that should leave nothing out.

Weighed: a separate claudes pin list, which would drift from agent view; and
naming every session to get past the pins, which `--pinned` replaces.

## 6. Safeguards are opt-in

By default, `restart` and `pause` act at once on every running background
session that isn't pinned. Waiting for busy sessions (`--wait`), asking first
(`--confirm`), and showing the plan only (`--dry-run`) are flags. `gc` and
`session gc` take `--confirm` and `--dry-run` too.

Why: claudes is an emergency tool, used when Claude Code has to restart now.
Default prompts and waits slow down the case it exists for.

Weighed: waiting and confirming by default.

## 7. Interactive sessions are opt-in, in two modes

Bulk commands leave interactive sessions alone and print the command that
resumes each one. `--interactive` stops and parks them, and `--interactive-bg`
resumes them in the background
([Interactive sessions](guide.md#interactive-sessions)).

Why: an interactive session belongs to a terminal claudes can't reach, and
stopping it by surprise loses the user's place.

Weighed: restarting interactive sessions by default, which ends work in a
terminal the user may be typing in.

## 8. A ping is a stop and a resume with a prompt

`--ping` parks a mid-turn session, stops it, waits for its process to go, and
resumes it with `claude --bg --resume <id> "<prompt>"`. An idle session restarts
with `claude respawn` and gets no prompt.

Why: `respawn` on a mid-turn session cuts its work short, and the session comes
back idle, waiting for input. The prompt tells it to carry on. Resuming a
session that still runs starts a copy, and a failed resume must not lose the
session. `--wait` waits only for busy sessions: a waiting one needs a person,
and a shell task such as a dev server may never end, so waiting for either would
run out the timeout.

Weighed: pinging every session, which wakes idle ones that had nothing to do;
and treating only `busy` as mid-turn, which restarts sessions mid-question and
kills their shell tasks unpinged.

## 9. The session running claudes goes last

The session claudes runs in is restarted, paused, or stopped after all the
others and after everything else is printed, and is never pinged. An interactive
host is parked and its command printed, not resumed.

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

A command that changes sessions takes the first of those steps that matches, and
more than one match there is an error. `ls` and `where` list every match instead
([Naming sessions](guide.md#naming-sessions)).

Why: names are what people remember, and a unique fragment is enough to type. A
short job id prefix must keep working when some name happens to contain it. An
ambiguous fragment must never pick a session at random, and a mistyped `--skip`
must not leave the session it meant to protect unprotected, so a skip that
matches nothing is an error.

Weighed: exact names only, which is tedious for long names; and fuzzy matching,
which can pick the wrong session. For `ls` and `where`, the stepwise rule hides
sessions: a name that starts with the word hides another that only contains it.
Matching the middle of an id is out, and the start of one needs four characters,
because random hex matches short words by accident: "2" or "db" would otherwise
pick a session by its id. Matching only running sessions is out too: a stopped
session named "api" let "restart api" fall through to a running "api-gw".

## 12. A session's version comes from its executable

The Claude Code version a running session runs comes from the path of its
process's executable, where the native installer names each version
(`…/claude/versions/2.1.326`), and otherwise from the job's `cliVersion` in
`~/.claude/jobs`.

Why: `claude agents --json` has no version, and `restart --outdated` needs one.
`cliVersion` alone is stale after a respawn: Claude Code keeps the old value
until the session takes a turn, so a freshly restarted idle session looked
outdated, and `--outdated` would restart it again. An npm install runs as
`node`, whose path names no version, so it still relies on `cliVersion`.

Weighed: `cliVersion` alone, which restarts sessions already on the new version.

## 14. Every `claude` call has a time limit

Listing, `--version`, and `rm` get one minute, and `stop`, `respawn`, and resume
get three. claudes also stops waiting for output five seconds after `claude`
exits, and a second Ctrl-C ends claudes at once. On Unix, SIGTERM and SIGHUP
count as Ctrl-C, so a tool's timeout or a closed terminal still gets the report.
The handoffs that give `claude` the terminal, such as `attach`, have no limit.

Why: a hung call would otherwise hang claudes for good, between a stop and its
resume. A background session that `claude` starts can inherit its output and
never close it, and cancelling the call does not end that wait.

Weighed: no limits, which is how the hang happened; and one short limit for
every call, which would cut off a slow but healthy stop.

## 15. Stopping an interactive session kills its process tree

`stop` and `--interactive` end an interactive session and every process it
started ([Interactive sessions](guide.md#interactive-sessions)). First, claudes
checks that the process is Claude Code: the native binary, or node running the
npm package, told apart by its command line. It also checks that the process
started no later than the session's `startedAt`, with two seconds of slack, and
before the sessions were listed. It signals a child only while its start time
still matches.

Why: Windows has no SIGTERM for console programs. Stopping only the session's
own process orphaned its shells and MCP servers. A reused PID must never be
killed. A session's process always starts first: measured, an interactive
session records `startedAt` about two seconds after its process starts, and a
background session runs in a spare process started ahead of time, minutes or
hours before. So only a process that started after the session is refused, and
the gap is never required to be small.

Weighed: signalling the process group on macOS and Linux, which misses a child
that started a group of its own. On Windows, which reads the command line
through `NtQueryInformationProcess`: reading it from the process's memory, which
needs more access than claudes asks for, and asking WMI, which is slow to start.
Sending Ctrl-Break first and killing after a grace period is untested against
Claude Code, and stays open in uvw/claudes#9.

## 16. Resume commands use the platform's shell

The printed resume commands are POSIX shell commands on macOS and Linux, and
PowerShell commands on Windows. Each session's name prints on its own line,
without control characters.

Why: Windows PowerShell 5.1 rejects `&&`, and cmd cannot change directories and
quote safely in one line. PowerShell reads curly single quotes as quotes, so a
directory name could otherwise end the string and add a command.

Weighed: printing both forms, which doubles the output for every session.

## 18. Settings live in one YAML file, and a typo stops claudes

`config.yaml` holds defaults for flags, and an unknown key or a bad value stops
every command but those that help fix it
([Configuration](reference.md#configuration)).

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
warns ([Compatibility](reference.md#compatibility)). A weekly workflow runs
`claudes doctor --cli-only --strict` and `TestLiveClaude` against the newest
Claude Code, and opens an issue when a check fails
([Tests](contributing.md#tests)).

Why: claudes decides whom to ping and how to stop a session from these values.
An unknown status read as idle would restart a session mid-turn without a ping.
A version number alone cannot tell whether a release changed them, so the checks
look at the data itself, and the version only prompts a look. The tests' fakes
encode the authors' model of the formats, so only the real output shows when
Claude Code changes them. A background job starts without a login and waits for
one, so the weekly check needs no API key. Such a job never takes a turn, so the
check cannot see `cliVersion`.

Weighed: refusing every untested version, which blocks claudes after each Claude
Code update, almost always for nothing; warning without stopping, which lets a
changed status cut work short; and checking only in `claudes doctor`, which
nobody runs before a bulk restart. For the weekly check, a `claudes doctor`
option that starts a job, which would add a flag users never need, and the full
`claudes doctor --strict`, which fails on the job's missing `cliVersion`.

## 20. Past sessions come from their transcripts

`claudes session` reads the transcripts under `~/.claude/projects`, since
`claude agents --json` knows only running and finished background sessions. It
reads the first 64 KB and the last 256 KB of each, and a whole file only when it
must, and it never changes a transcript. `session mv` copies one and keeps the
original. For a stopped background session, it also removes the job with
`claude rm` and keeps the job's saved options, `respawnFlags`, for the next
`session resume --bg` ([Past sessions](guide.md#past-sessions)).

Why: Claude Code writes the title and the last prompt again every few turns, so
both ends of a file name the session. Reading the ends lists 50 sessions in
about 12 ms, so no index is needed to list them. Counting what a session did
needs every line, and a session with more than 2 MB in it is not trivial anyway:
reading the 170 transcripts under 2 MB here adds about a second.

Copying instead of moving keeps the session resumable from its old directory if
the copy goes wrong, and a running session keeps writing to its old transcript
anyway. `claude --bg --resume` wakes an existing job in the directory it started
in, wherever it runs from. A live check removed the job and ran the same command
from the new directory. It resumed the session itself under the same id,
continued the copy's transcript, and saved the options passed to the new job.
Passing options to a job that still exists starts a copy instead. Claude Code
writes `respawnFlags` for every job, if only as `[]`, so a missing key means the
file changed, and `mv` keeps that job rather than lose options such as
`--model`.

Weighed: reading each transcript in full, which reads hundreds of megabytes to
list sessions; moving the transcript, which loses the session if the new
directory is wrong; rewriting the directory recorded inside the transcript,
which `claude --resume` does not need; and keeping the job, which left
`session resume --bg` refusing the session until `claudes gc` removed it and its
options with it.

## 22. Few dependencies, and our own plumbing

claudes keeps its modules few, and keeps atomic writes, the lock directory,
platform directories, shell quoting, and process checks in `internal/`.

Why: each of those must match what Claude Code does byte for byte, such as the
`pins.json.lock` directory proper-lockfile makes, or is a few dozen lines. The
packages that cover them either behave differently, such as a lock file in place
of a lock directory, or are archived. Every module is under a license
`hack/notices.go` allows.

Weighed: `google/renameio` and `natefinch/atomic` for writes, `gofrs/flock` for
locks, `adrg/xdg` for directories, `kballard/go-shellquote` for quoting, and
`hashicorp/go-version`, whose MPL license is outside the allowed ones.

## 23. One claudes changes sessions at a time, and exit codes say why one stopped

Commands that stop, start, remove, or pin sessions take a lock directory in
claudes' state directory for the whole run, and a second run fails at once with
exit 5 ([Safety](guide.md#safety)). The lock names the PID and start time of the
claudes that holds it, and refreshes while it is held. A stale lock is broken
only once its claudes has ended, or when it names none. claudes releases it
before it stops its own session. Each kind of failure has its own exit code
([Exit codes](reference.md#exit-codes)).

Why: two restarts at once could stop the same session twice, and a resume racing
a pause could start a copy. claudes is an emergency tool, so failing at once
beats waiting quietly behind another run. A claudes that is suspended, such as
on a closed laptop lid, stops refreshing its lock, so a lock that only went
stale would let a second run start while the first wakes to finish. An agent
that runs claudes needs to tell "try another name" from "nothing changed"
without parsing English.

Weighed: a lock per session, which still lets two runs plan against each other's
half-done work; a lock judged by its age alone, which a suspend breaks; waiting
for the lock, which hides a stuck run; and one failure code, which is what
claudes had.

## 25. claudes reads agent teams from their files, and guards their leads

claudes reads Claude Code's agent teams from
`~/.claude/teams/<team>/config.json` and the task lists under `~/.claude/tasks`,
both documented. A lead whose config lists members besides itself has teammates
at work: bulk `restart` and `pause` skip it, named ones go ahead with a warning
that names the teammates, and `--ping` prompts it, so it can start them again
([Agent teams](guide.md#agent-teams)). `gc --teams` removes the team directories
and task lists Claude Code leaves behind.

Why: stopping a lead stops its in-process teammates, which Claude Code cannot
resume, and `claude agents` lists only the lead, so nothing else shows the loss
coming. A live test showed it: a restarted lead's two teammates died mid-task,
and the pinged lead started new ones from scratch. Claude Code keeps a team
directory for every session once teams are on, and leaves them behind.

Weighed: refusing to restart a lead at all, which blocks the emergency use;
reading the undocumented `inFlight` field of a job's `state.json`, which the
team config makes unnecessary; and leaving team directories to Claude Code,
which kept them for weeks here.

## 27. session gc moves to the trash, and never touches a session in use

`claudes session gc` moves past sessions, with everything Claude Code keeps for
each, to the system trash, and `--purge` deletes them for good. Without flags it
takes only empty sessions, older copies, and leftover directories, and it judges
a session empty or trivial only on record types it knows. It never removes a
running, listed, or parked session, the session running claudes, or, without
targets, anything that changed in the last day. `--sidecars` trims a session's
saved outputs, and with `--older-than` its subagent transcripts, without
removing the session ([Past sessions](guide.md#past-sessions)). `gc --teams` and
`session mv --rm` move what they remove to the trash too, and take `--purge`.

Why: a transcript is the only record of a session's work, so a mistaken removal
must be recoverable, unlike `claudes gc`, which only drops a job from agent
view's list. Empty sessions, superseded copies, and orphaned directories hold
nothing anyone can resume, so they are safe to pick by default. Claude Code adds
and renames record types often, and empty is a default kind, so a type claudes
has not seen keeps the session until claudes learns it. A missing directory can
be a disk that is not attached, and age says nothing about value, so those wait
for a flag. So does `--trivial`: a one-prompt answer can still be worth keeping,
though one with no tool call holds only what the model already knew. Which of
the one-prompt sessions with tool calls to keep takes reading the prompt, so
claudes leaves that to the user or their agent (decision 28).

A test resumed sessions with each part of the sidecar moved away: both resumed
with their context, since the transcript keeps a preview of each saved output
and each subagent's answer. Without `tool-results`, reading a saved output again
fails; without `subagents`, continuing a past subagent fails. Subagents held 1.3
GB against 227 MB of saved outputs here, so they go too, but only for sessions
old enough that nobody continues their subagents. A session that changed in the
last day may be starting, before its transcript says what it is.

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
thing on every command: `-n` is always a dry run, for one. A flag that overrides
a safeguard never gets one, such as `--force`, `--purge`, `--pinned`, or
`--no-redact`. `TestEachShortFlagMeansOneThingEverywhere` walks every command
and enforces both rules.

Why: claudes is typed in a hurry, after an update or before a reboot, so the
common flags should be short. A letter that changes meaning between commands
turns muscle memory into a mistake. An override is the one flag whose typing
should take a moment, and `-f` already means follow.

Weighed: a short form for every flag, which runs out of letters and spends them
on flags almost nobody types; and `-f` for `--force`, as many tools do, which
clashes with `logs -f`.

## 32. session search scans every transcript, with no index

`session search` reads every transcript in full each time it runs, one worker
per CPU, and keeps nothing between runs. Before it parses a line, it looks for
the query's words in the raw bytes, ignoring ASCII case, and it skips tool
results and other record types by their `"type"` key, so most lines are never
parsed.

Why: a scan is fast enough. On a 16-core computer with 310 transcripts taking
2.3 GB, a warm search that matches nothing takes 0.35 to 0.5 s, about as long as
ripgrep takes to find nothing in the same files. A common word takes 0.7 to 0.8
s. A scan is never stale, needs no storage, and adds no dependency. An index
would have to follow transcripts that Claude Code writes, moves, and compacts
while sessions run, and would add a database to a tool that keeps a few small
files.

Weighed: an SQLite full-text index, which answers in milliseconds but needs a
dependency, a rebuild after every change, and a way to notice what changed; an
index of claudes' own, with the same upkeep; and parsing every user and
assistant record, which takes three times as long. An index stays possible if
transcripts outgrow the scan.

## 33. Windows replaces a file with POSIX rename semantics first

On Windows, an atomic write renames its temp file over the target with
`os.Root.Rename`. Go tries `FILE_RENAME_POSIX_SEMANTICS` first, and a plain
rename when that fails for any reason. A target that stays held gets nine
retries, a little under a second in all.

Why: agent view reads `pins.json` while claudes writes it, and a plain rename
waits until agent view closes the file. A POSIX rename replaces a file other
processes hold open, as long as each opened it with delete sharing, as Node and
Bun, which run Claude Code, do. It needs Windows 10 1607 or later and a file
system that has it, such as NTFS, and the errors from those that lack it vary,
so any error falls back. Readers still see the old file or the new one. Go keeps
both attempts in the standard library, so claudes carries no `FILE_RENAME_INFO`
code of its own.

Weighed: POSIX semantics alone, which fail where the file system lacks them;
calling `SetFileInformationByHandle` with `FILE_RENAME_INFO` by hand, which
makes the same two attempts with unsafe code claudes would own; and
`FILE_RENAME_FLAG_IGNORE_READONLY_ATTRIBUTE`, which would overwrite a file the
owner made read-only and needs Windows 10 1809.

## 34. MCP control starts read-only, and never prints a server's secrets

`claudes mcp ls` and `mcp profiles` only read, and never print a server's
command, arguments, environment, headers, or URL, in text or JSON. A profile is
a list of the servers that stay on ([MCP servers](guide.md#mcp-servers)).

Why: deciding which servers a session loads is one of the jobs around running
Claude Code sessions, the README's first goal. Claude Code spreads the answer
over `~/.claude.json`, settings files, `.mcp.json` files, and plugins, so the
read side is useful alone. It also comes first because writing `~/.claude.json`
while Claude Code runs needs tests against a live Claude Code. Server entries
carry tokens in their arguments, environment, headers, and URLs, and a listing
ends up in terminals, logs, and agent transcripts, so claudes keeps only the
name and the transport. An on-list says what a kind of work needs, and stays
right when a new server is added: the new one is off in every profile until
someone adds it.

Weighed: printing the command or URL with secrets masked, which still leaks
tokens claudes cannot recognize, such as one inside a path; an off-list per
profile, which would let every new server in; and building `mcp on`, `mcp off`,
and launch profiles in the same change, before the live tests that show how
Claude Code picks up a change while it runs.

## 35. session search masks secrets in what it prints

`session search` masks token-shaped text in the session names and snippets it
prints, in text and in `-o json`, while matching still reads the text as written
([Searching past sessions](guide.md#searching-past-sessions)). `--no-redact`
prints everything, and has no short form (decision 30).
`internal/transcripts/redact.go` holds the rules.

Why: what search prints lands in terminals, logs, and agent transcripts, and the
agent skill tells agents to run it. `--ran curl` would otherwise copy an
`Authorization: Bearer` header or an `export TOKEN=` line into a new transcript.
`mcp ls` keeps secrets out by printing only names (decision 34), but search
exists to print what was said, so it masks the secrets it can recognize instead.
Masking changes only what is printed, and never stops a command or asks first,
so it is on by default, unlike the safeguards of decision 6. Each field is
masked in full before its snippet is cut, so a secret the cut splits is still
found. A rule runs only on text that holds its hint, such as `://` or `key`, and
only for the few records a session keeps as its best. A search in which every
record matches and holds those hints takes about 2% longer.

Weighed: printing text as it is, which copies secrets into transcripts; masking
only with a flag, which agents would not pass; masking only the snippet after
the cut, which misses a secret whose start the cut drops; and a generic
high-entropy rule, which would also mask git hashes and UUIDs. A secret of a
shape the rules do not know still prints.
