# Decisions

Each entry names a settled choice about how claudes behaves or stays safe, links
to where the rule is documented, and says why and what else was weighed.
[contributing.md](contributing.md#docs) says what belongs here and how to keep
it.

## Index

| No.                                                                                   | Decision                                                                        |
| ------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------- |
| [3](#3-anything-else-goes-to-claude-agents)                                           | Anything else goes to `claude agents`                                           |
| [5](#5-pins-are-agent-views-pins-and-they-protect-sessions)                           | Pins are agent view's pins, and they protect sessions                           |
| [6](#6-safeguards-are-opt-in)                                                         | Safeguards are opt-in                                                           |
| [7](#7-interactive-sessions-are-opt-in-in-two-modes)                                  | Interactive sessions are opt-in, in two modes                                   |
| [8](#8-a-ping-is-a-stop-and-a-resume-with-a-prompt)                                   | A ping is a stop and a resume with a prompt                                     |
| [9](#9-the-session-running-claudes-goes-last)                                         | The session running claudes goes last                                           |
| [10](#10-pause-saves-the-park-file-before-it-stops-anything)                          | Pause saves the park file before it stops anything                              |
| [11](#11-targets-match-exactly-then-by-prefix-then-by-part-of-a-name)                 | Targets match exactly, then by prefix, then by part of a name                   |
| [12](#12-a-sessions-version-comes-from-its-executable)                                | A session's version comes from its executable                                   |
| [14](#14-every-claude-call-has-a-time-limit)                                          | Every `claude` call has a time limit                                            |
| [15](#15-stopping-an-interactive-session-kills-its-process-tree)                      | Stopping an interactive session kills its process tree                          |
| [16](#16-resume-commands-use-the-platforms-shell)                                     | Resume commands use the platform's shell                                        |
| [18](#18-settings-live-in-one-yaml-file-and-a-typo-stops-claudes)                     | Settings live in one YAML file, and a typo stops claudes                        |
| [19](#19-unfamiliar-claude-code-output-stops-writes)                                  | Unfamiliar Claude Code output stops writes                                      |
| [20](#20-past-sessions-come-from-their-transcripts)                                   | Past sessions come from their transcripts                                       |
| [22](#22-few-dependencies-and-our-own-plumbing)                                       | Few dependencies, and our own plumbing                                          |
| [23](#23-one-claudes-changes-sessions-at-a-time-and-exit-codes-say-why-one-stopped)   | One claudes changes sessions at a time, and exit codes say why one stopped      |
| [25](#25-claudes-reads-agent-teams-from-their-files-and-guards-their-leads)           | claudes reads agent teams from their files, and guards their leads              |
| [27](#27-session-gc-moves-to-the-trash-and-never-touches-a-session-in-use)            | session gc moves to the trash, and never touches a session in use               |
| [28](#28-session-triage-sorts-and-the-agent-judges)                                   | session triage sorts, and the agent judges                                      |
| [29](#29-claudes-is-a-tool-for-claude-code-and-nothing-else)                          | claudes is a tool for Claude Code, and nothing else                             |
| [30](#30-short-flags-mean-one-thing-and-safeguards-stay-long)                         | Short flags mean one thing, and safeguards stay long                            |
| [32](#32-session-search-scans-every-transcript-with-no-index)                         | session search scans every transcript, with no index                            |
| [33](#33-windows-replaces-a-file-with-posix-rename-semantics-first)                   | Windows replaces a file with POSIX rename semantics first                       |
| [34](#34-mcp-control-never-prints-a-servers-secrets)                                  | MCP control never prints a server's secrets                                     |
| [35](#35-session-search-masks-secrets-in-what-it-prints)                              | session search masks secrets in what it prints                                  |
| [36](#36-claudes-records-what-it-trashes-and-restores-without-replacing)              | claudes records what it trashes, and restores without replacing                 |
| [37](#37-claudes-reaches-into-a-running-session-through-a-mod)                        | claudes reaches into a running session through a mod                            |
| [38](#38-claudes-writes-only-a-directorys-disabledmcpservers-with-a-compare-and-swap) | claudes writes only a directory's `disabledMcpServers`, with a compare-and-swap |
| [39](#39-a-profile-launches-through-allowedmcpservers-matched-by-url-or-command)      | A profile launches through `allowedMcpServers`, matched by URL or command       |

## 3. Anything else goes to `claude agents`

Anything that is not a claudes command runs `claude agents` with the same
arguments ([Commands](guide.md#commands)).

Why: claudes stands in for `claude agents`, so agent view's flags must keep
working through it. A new claudes command hides a `claude agents` argument of
the same name, and `claudes -- ARGS` gets around that.

Weighed: a `claudes agents` subcommand, one more word for the most common use.

## 5. Pins are agent view's pins, and they protect sessions

claudes shares agent view's `pins.json`, and bulk commands skip pinned sessions
([Safety](guide.md#safety)).

Why: with one pin list, a session pinned in agent view is protected in claudes
too, with nothing to keep in sync. `--pinned` takes them all at once, for a
reboot or an update that should leave nothing out.

Weighed: a pin list of claudes' own, which would drift from agent view's; and
naming every session to get past the pins.

## 6. Safeguards are opt-in

Waiting, confirming, and dry runs are flags, never defaults
([Common flags](guide.md#common-flags)).

Why: claudes is an emergency tool, used when Claude Code has to restart now.
Default prompts and waits slow down the case it exists for.

Weighed: waiting and confirming by default.

## 7. Interactive sessions are opt-in, in two modes

Bulk commands leave interactive sessions alone unless a flag says otherwise
([Interactive sessions](guide.md#interactive-sessions)).

Why: an interactive session belongs to a terminal claudes can't reach, and
stopping it by surprise loses the user's place.

Weighed: restarting interactive sessions by default, which ends work in a
terminal the user may be typing in.

## 8. A ping is a stop and a resume with a prompt

With `--ping`, a mid-turn session is parked, stopped, and resumed with a prompt,
and an idle one gets `claude respawn` ([Safety](guide.md#safety)).

Why: `respawn` cuts a mid-turn session's work short, and it comes back idle; the
prompt tells it to carry on. Resuming a session that still runs starts a copy,
so claudes waits for the process to go, and it parks first so a failed resume
loses nothing. `--wait` waits only for busy sessions: a waiting one needs a
person, and a shell task such as a dev server may never end.

Weighed: pinging every session, which wakes idle ones that had nothing to do;
and treating only `busy` as mid-turn, which restarts sessions mid-question and
kills their shell tasks unpinged.

## 9. The session running claudes goes last

The session claudes runs in goes after every other session and all the output,
and is never pinged ([Safety](guide.md#safety)).

Why: stopping it ends the command, so nothing after it would happen, including
the resume commands and failures the user needs. So an interactive host is
parked and its command printed, not resumed.

Weighed: leaving the host alone, which keeps it on an old Claude Code after a
bulk restart.

## 10. Pause saves the park file before it stops anything

`pause` records the sessions it will stop before it stops any of them
([Files](reference.md#files)).

Why: a crash or a reboot halfway through a pause must not lose the list. A stop
can take effect and still report an error, so a session whose stop failed leaves
the list only once it is seen still running.

Weighed: writing the file after the stops, which loses it on a crash.

## 11. Targets match exactly, then by prefix, then by part of a name

A command that changes sessions takes the first step that matches, and several
matches there are an error; `ls` and `where` list every match
([Naming sessions](guide.md#naming-sessions)).

Why: people remember names, and a unique fragment is enough to type. A short job
id prefix must keep working when a name happens to contain it. An ambiguous
fragment must never pick a session at random, and a mistyped `--skip` must not
leave its session unprotected, so a skip that matches nothing is an error.

Weighed: exact names only, tedious for long names; and fuzzy matching, which can
pick the wrong session. For `ls` and `where`, the stepwise rule would hide a
name that contains the word behind one that starts with it. Ids match only from
the start, and from four characters, because random hex matches short words: "2"
or "db" would otherwise pick a session by its id. Matching only running sessions
is out: a stopped session named "api" let "restart api" fall through to a
running "api-gw".

## 12. A session's version comes from its executable

A running session's Claude Code version comes from its executable's path, where
the native installer names the version, or else from the job's `cliVersion`.

Why: `claude agents --json` has no version, and `restart --outdated` needs one.
Claude Code keeps the old `cliVersion` after a respawn until the session takes a
turn, so a freshly restarted idle session looked outdated, and `--outdated`
would restart it again. An npm install runs as `node`, whose path names no
version, so it still relies on `cliVersion`.

Weighed: `cliVersion` alone, which restarts sessions already on the new version.

## 14. Every `claude` call has a time limit

Every `claude` call claudes waits on has a time limit, set in
`internal/claude/client.go`; the handoffs that give `claude` the terminal, such
as `attach`, have none.

Why: a hung call would otherwise hang claudes for good, between a stop and its
resume. A stop can be slow and still healthy, so it gets three minutes where a
listing gets one. A background session that `claude` starts can inherit its
output and never close it, and canceling the call does not end that wait. So
claudes stops reading five seconds after `claude` exits.

Weighed: no limits, which is how the hang happened; and one short limit for
every call, which would cut off a slow but healthy stop.

## 15. Stopping an interactive session kills its process tree

`stop` and `--interactive` end an interactive session and every process it
started ([Interactive sessions](guide.md#interactive-sessions)).

Why: Windows has no SIGTERM for console programs, and stopping only the
session's own process orphaned its shells and MCP servers. A reused PID must
never be killed. So claudes checks that the process is Claude Code, the native
binary or node running the npm package. It also checks that the process started
before the listing and no later than the session's `startedAt`, with two seconds
of slack. It signals a child only while its start time still matches. Measured,
an interactive session records `startedAt` about two seconds after its process
starts, and a background session runs in a spare process started minutes or
hours ahead. So the check refuses only a process that started after the session,
and never needs the gap to be small.

Weighed: signaling the process group on macOS and Linux, which misses a child
that started a group of its own. On Windows, where claudes reads the command
line through `NtQueryInformationProcess`: reading it from the process's memory,
which needs more access, and asking WMI, which is slow to start. Ctrl-Break
first and a kill after a grace period is untested against Claude Code, and stays
open in uvw/claudes#9.

## 16. Resume commands use the platform's shell

Printed resume commands are POSIX shell on macOS and Linux, and PowerShell on
Windows ([Interactive sessions](guide.md#interactive-sessions)).

Why: Windows PowerShell 5.1 rejects `&&`, and cmd cannot change directories and
quote safely in one line. PowerShell reads curly single quotes as quotes, so a
directory name could otherwise end the string and add a command.

Weighed: printing both forms, which doubles the output for every session.

## 18. Settings live in one YAML file, and a typo stops claudes

An unknown key or a bad value in `config.yaml` stops every command but those
that help fix it ([Configuration](reference.md#configuration)).

Why: YAML is short to write by hand and reads the same on every platform. A typo
claudes ignored would leave a setting silently off, which is worse for an
emergency tool than refusing to run.

Weighed: TOML, which needs a second parser for little gain; environment
variables alone, which do not persist across shells; and ignoring unknown keys,
which hides typos.

## 19. Unfamiliar Claude Code output stops writes

A command that changes sessions or pins stops on values it does not recognize,
and an untested version only warns
([Compatibility](reference.md#compatibility)). A weekly workflow runs
`claudes doctor --cli-only --strict` and `TestLiveClaude` against the newest
Claude Code.

Why: claudes decides whom to ping and how to stop a session from these values.
An unknown status read as idle would restart a session mid-turn without a ping.
A version number cannot tell whether a release changed them, so the checks look
at the data, and the version only prompts a look. The tests' fakes encode the
authors' model of the formats, so only real output shows a change. A background
job starts without a login and waits for one, so the weekly check needs no API
key, but it never takes a turn, so it has no `cliVersion`.

Weighed: refusing every untested version, which blocks claudes after each Claude
Code update, almost always for nothing; warning without stopping, which lets a
changed status cut work short; and checking only in `claudes doctor`, which
nobody runs before a bulk restart. For the weekly check: a `claudes doctor`
option that starts a job, a flag users never need; and the full
`claudes doctor --strict`, which fails on the missing `cliVersion`.

## 20. Past sessions come from their transcripts

`claudes session` reads past sessions from their transcripts and never changes
one, and `session mv` copies a session rather than moving it
([Past sessions](guide.md#past-sessions)).

Why: `claude agents --json` knows only running and finished background sessions.
Claude Code writes the title and the last prompt again every few turns, so both
ends of a file name the session. claudes reads the first 64 KB and the last 256
KB, which lists 50 sessions in about 12 ms with no index. Counting what a
session did needs every line, and a session over 2 MB is not trivial anyway:
reading the 170 transcripts under 2 MB here adds about a second.

A copy keeps the session resumable from its old directory if anything goes
wrong, and a running session keeps writing to its old transcript anyway.
`claude --bg --resume` wakes an existing job in the directory it started in, so
`mv` removes a stopped job and keeps its saved options. A live check removed the
job and ran the same command from the new directory: it resumed the session
under the same id, continued the copy's transcript, and saved the options
passed. Passing options to a job that still exists starts a copy instead. Claude
Code writes `respawnFlags` for every job, if only as `[]`, so a missing key
means the format changed, and `mv` keeps that job rather than lose options such
as `--model`.

Weighed: reading each transcript in full, hundreds of megabytes to list
sessions; moving the transcript, which loses the session if the new directory is
wrong; rewriting the directory inside the transcript, which `claude --resume`
does not need; and keeping the job, which left `session resume --bg` refusing
the session until `claudes gc` removed the job and its options.

## 22. Few dependencies, and our own plumbing

claudes keeps its modules few, and keeps atomic writes, the lock directory,
platform directories, shell quoting, and process checks in `internal/`.

Why: each must match what Claude Code does byte for byte, such as the
`pins.json.lock` directory proper-lockfile makes, or is a few dozen lines. The
packages that cover them behave differently, such as a lock file in place of a
lock directory, or are archived. Every module is under a license
`hack/notices.go` allows.

Weighed: `google/renameio` and `natefinch/atomic` for writes, `gofrs/flock` for
locks, `adrg/xdg` for directories, `kballard/go-shellquote` for quoting, and
`hashicorp/go-version`, whose MPL license is outside the allowed ones.

## 23. One claudes changes sessions at a time, and exit codes say why one stopped

Commands that change sessions hold one lock for the whole run, a second run
fails at once with exit 5, and each kind of failure has its own exit code
([Safety](guide.md#safety), [Exit codes](reference.md#exit-codes)).

Why: two restarts at once could stop the same session twice, and a resume racing
a pause could start a copy. claudes is an emergency tool, so failing at once
beats waiting quietly behind another run. A suspended claudes, such as on a
closed laptop lid, stops refreshing its lock, so the lock names its holder and
breaks only once that claudes has ended. An agent that runs claudes needs to
tell "try another name" from "nothing changed" without parsing English.

Weighed: a lock per session, which still lets two runs plan against each other's
half-done work; a lock judged by its age alone, which a suspend breaks; waiting
for the lock, which hides a stuck run; and one failure code, which is what
claudes had.

## 25. claudes reads agent teams from their files, and guards their leads

claudes reads agent teams from the team configs and task lists Claude Code
documents, bulk commands hold back a lead whose teammates are at work, and
`gc --teams` removes what Claude Code leaves behind
([Agent teams](guide.md#agent-teams)).

Why: stopping a lead stops its in-process teammates, which Claude Code cannot
resume, and `claude agents` lists only the lead, so nothing else shows the loss
coming. In a live test, a restarted lead's two teammates died mid-task, and the
pinged lead started new ones from scratch. Claude Code keeps a team directory
for every session once teams are on, and leaves them behind.

Weighed: refusing to restart a lead at all, which blocks the emergency use;
reading the undocumented `inFlight` field of a job's `state.json`, which the
team config makes unnecessary; and leaving team directories to Claude Code,
which kept them for weeks here.

## 27. session gc moves to the trash, and never touches a session in use

`session gc` moves past sessions to the system trash, as `gc --teams` and
`session mv --rm` do, and never removes one in use
([Past sessions](guide.md#past-sessions)).

Why: a transcript is the only record of a session's work, so a mistaken removal
must be recoverable; `claudes gc` only drops a job from agent view's list. Empty
sessions, superseded copies, and orphaned directories hold nothing to resume, so
they are safe defaults. Claude Code adds and renames record types often, so a
type claudes has not seen keeps a session out of the empty default. A missing
directory can be a detached disk, and age says nothing about value, so those
wait for a flag. So does `--trivial`: a one-prompt answer can be worth keeping,
though one with no tool call holds only what the model already knew. Which
one-prompt sessions with tool calls to keep takes reading the prompt, so claudes
leaves that to the user or their agent (decision 28). A session that changed in
the last day may be starting, before its transcript says what it is.

`--sidecars` trims instead. A test resumed sessions with each part of the
sidecar moved away: both kept their context, since the transcript keeps a
preview of each saved output and each subagent's answer. Without `tool-results`,
reading a saved output again fails; without `subagents`, continuing a past
subagent fails. Subagents held 1.3 GB against 227 MB of saved outputs here, so
they go too, but only for sessions old enough that nobody continues their
subagents.

Weighed: deleting outright with a `--dry-run` preview, as `gc` does, which
cannot undo a mistake; previewing by default with `--yes` to act, against
decision 6; and a trash directory of claudes' own, which the system's tools
cannot list or restore from.

## 28. session triage sorts, and the agent judges

`session triage` sorts past sessions from facts it can count, changes nothing,
and leaves its review group to whoever runs it
([Past sessions](guide.md#past-sessions)).

Why: the same shape fits a commit message written once and a pull request review
whose findings live nowhere else, and only reading the prompt tells them apart.
An agent the user already runs can read it, so claudes stays offline and
deterministic and sends no transcript text anywhere new. The agent skill tells
that agent how to judge, and to ask before removing any. Above 20 tool calls, a
one-prompt session did enough work to keep.

Weighed: `session triage --agent`, which would run `claude -p` with a schema to
fill in the verdicts, at the cost of a model call, a time limit (decision 14),
and transcript text in a new process; and heuristics on the prompt's words,
which would remove the review that looked like a lookup.

## 29. claudes is a tool for Claude Code, and nothing else

claudes manages Claude Code's sessions, one job per command, and neither
orchestrates agents nor supports other coding agents
([Goals and non-goals](../README.md#goals-and-non-goals)).

Why: claudes reads interfaces Claude Code does not promise to keep (decision
19), and every write depends on checks made for that output. Each other agent
would need its own readers, checks, and test fakes, making claudes many tools in
one. Orchestration is a product of its own, and Claude Code already has agent
teams and agent view for it. Staying small keeps claudes safe to run against
live work, the job it exists for.

Weighed: an adapter layer that would let claudes drive other agents' sessions;
and scheduling or chaining sessions, such as starting one when another finishes,
which is orchestration under another name.

## 30. Short flags mean one thing, and safeguards stay long

Each one-letter flag means the same on every command, and a flag that overrides
a safeguard, such as `--force`, has none.
`TestEachShortFlagMeansOneThingEverywhere` enforces both.

Why: claudes is typed in a hurry, after an update or before a reboot, so common
flags should be short. A letter that changes meaning between commands turns
muscle memory into a mistake. An override is the one flag whose typing should
take a moment.

Weighed: a short form for every flag, which runs out of letters on flags almost
nobody types; and `-f` for `--force`, as many tools do, which clashes with
`logs -f`.

## 32. session search scans every transcript, with no index

`session search` reads every transcript on each run and keeps nothing between
runs ([Searching past sessions](guide.md#searching-past-sessions)).

Why: a scan is fast enough. It looks for the words in the raw bytes before it
parses a line, and skips records by their `"type"` key, so most lines are never
parsed. On a 16-core computer with 310 transcripts taking 2.3 GB, a warm search
that matches nothing takes 0.35 to 0.5 s, about as long as ripgrep takes to find
nothing in the same files, and a common word takes 0.7 to 0.8 s. A scan is never
stale, needs no storage, and adds no dependency. An index would have to follow
transcripts that Claude Code writes, moves, and compacts while sessions run.

Weighed: an SQLite full-text index, which answers in milliseconds but needs a
dependency, a rebuild after every change, and a way to notice what changed; an
index of claudes' own, with the same upkeep; and parsing every user and
assistant record, which takes three times as long. An index stays possible if
transcripts outgrow the scan.

## 33. Windows replaces a file with POSIX rename semantics first

On Windows, an atomic write renames with POSIX semantics first, and falls back
to a plain rename on any error (`internal/files`).

Why: agent view reads `pins.json` while claudes writes it, and a plain rename
waits until agent view closes the file. A POSIX rename replaces a file other
processes hold open, if each opened it with delete sharing, as Node and Bun,
which run Claude Code, do. It needs Windows 10 1607 or later and a file system
that has it, such as NTFS, and the errors from those that lack it vary, so any
error falls back. Readers still see the old file or the new one.
`os.Root.Rename` makes both attempts, so claudes owns no `FILE_RENAME_INFO`
code.

Weighed: POSIX semantics alone, which fail where the file system lacks them;
calling `SetFileInformationByHandle` with `FILE_RENAME_INFO` by hand, the same
two attempts in unsafe code claudes would own; and
`FILE_RENAME_FLAG_IGNORE_READONLY_ATTRIBUTE`, which would overwrite a file the
owner made read-only and needs Windows 10 1809.

## 34. MCP control never prints a server's secrets

`mcp ls` and `mcp profiles` never print a server's command, arguments,
environment, headers, or URL, in text or JSON, and a profile lists the servers
that stay on ([MCP servers](guide.md#mcp-servers)). `mcp on` and `mcp off`
change only a directory's `disabledMcpServers` (decision 38).

Why: deciding which servers a session loads is one of the jobs around running
Claude Code sessions, the README's first goal. Claude Code spreads the answer
over `~/.claude.json`, settings files, `.mcp.json` files, and plugins, so the
read side is useful alone. Server entries carry tokens in their arguments,
environment, headers, and URLs, and a listing ends up in terminals, logs, and
agent transcripts. An on-list says what a kind of work needs, and a new server
stays off in every profile until someone adds it.

Weighed: printing the command or URL with secrets masked, which still leaks
tokens claudes cannot recognize, such as one inside a path; and an off-list per
profile, which would let every new server in.

## 35. session search masks secrets in what it prints

`session search` masks token-shaped text in what it prints, and `--no-redact`
turns that off ([Searching past sessions](guide.md#searching-past-sessions)).

Why: what search prints lands in terminals, logs, and agent transcripts, and the
agent skill tells agents to run it. `--ran curl` would otherwise copy an
`Authorization: Bearer` header or an `export TOKEN=` line into a new transcript.
`mcp ls` prints only names (decision 34), but search exists to print what was
said, so it masks the secrets it can recognize. Masking changes only output and
never stops a command, so it is on by default, unlike the safeguards of
decision 6. Each field is masked in full before its snippet is cut, so a secret
the cut splits is still found. A rule runs only on text that holds its hint,
such as `://` or `key`, and only for the few records a session keeps as its
best. A search in which every record matches and holds those hints takes about
2% longer.

Weighed: printing text as it is, which copies secrets into transcripts; masking
only with a flag, which agents would not pass; masking only the snippet after
the cut, which misses a secret whose start the cut drops; and a generic
high-entropy rule, which would also mask git hashes and UUIDs. A secret of a
shape the rules do not know still prints.

## 36. claudes records what it trashes, and restores without replacing

claudes records each file it moves to the trash, and `session restore` puts a
session's files back from that record, refusing outright when any file's place
is taken or the file has left the trash
([Restoring past sessions](guide.md#restoring-past-sessions)).

Why: decision 27 sends removals to the trash so a mistake can be undone, but on
macOS that undo was guesswork. Finder cannot put back what another program
trashed, the Trash holds numbered copies of UUID-named files, and one session
comes from four directories. Linux uses the same record rather than its
`.trashinfo` files, so every platform that says where a file went takes one code
path. `gc --teams` and `session mv --rm` record too, since a team directory or
an original transcript is as hard to find in the Trash. A taken path may hold
newer work, so restore never replaces a file, and it checks every file before it
moves any, so a refusal leaves nothing half restored. The record keeps the
`moves.json` entries `session gc` drops, so a moved session still resumes where
it moved. Restore is its own command because it lists without a target, writes
where `gc` deletes, and shares none of `session gc`'s flags.

Weighed: `session gc --restore`, as first proposed, a flag that would turn off
every other `session gc` flag and hide what `gc --teams` and `session mv --rm`
trashed; reading `.trashinfo` on Linux and searching the Recycle Bin's `$I`
files on Windows, which are two more code paths; restoring what it can when a
file is missing or a path is taken, which leaves a session half back; and a
trash directory of claudes' own, which decision 27 turned down.

## 37. claudes reaches into a running session through a mod

The claudes mod is a Claude Code plugin in `plugins/claudes-mod`. It runs the
slash commands claudes asks for inside a running session once the session is
idle, and only those it allows ([The claudes mod](guide.md#the-claudes-mod)).
claudes asks through one request file per session, in a directory only the user
can read. `claudes mod install` installs it with `claude plugin` commands.

Why: claudes could reach a running session only from outside, by stopping,
respawning, or killing it, which cuts off a turn and drops what the session
loaded. Claude Code's mod API queues `$.command.run` until the session is idle,
and live tests on throwaway sessions showed it leaves a permission prompt and a
typed draft alone, in interactive sessions too. Files need no server, so claudes
stays one binary with nothing running behind it, and each session reads only its
own requests. An allowlist in the mod means a request file can run nothing but
what claudes was built to ask for.

Weighed: typing the command into `claude attach`, which in the tests approved an
open permission prompt and mixed the command into a half-typed draft; stopping
and resuming the session, which still works with `--restart` but interrupts
work; a socket or HTTP listener in the mod, which every session would hold open;
and writing the plugin into Claude Code's plugin directory by hand, which
`claude plugin` does with its own checks. The mod does not load in sessions
started with `--bare`, `--safe-mode`, or `disableAllHooks`, or under a managed
`allowManagedHooksOnly`, so every command that uses it has a fallback.

## 38. claudes writes only a directory's `disabledMcpServers`, with a compare-and-swap

`mcp on` and `mcp off` go through the session's claudes mod, which runs Claude
Code's own `/mcp`, and claudes only reads `~/.claude.json` to check the result.
With `--restart` and `--in`, claudes writes one value itself: the
`disabledMcpServers` list of the directory's entry
([Turning servers on and off](guide.md#turning-servers-on-and-off)). It splices
the new list into the file's bytes, laid out as Claude Code's
`JSON.stringify(config, null, 2)` lays it out. It checks that the result parses
to the old file with only that list changed. It replaces the file only if it
still holds what claudes read, and tries three times.

Why: `~/.claude.json` holds every project's settings, the user's MCP servers
with their tokens, and Claude Code's own state, and Claude Code rewrites it many
times a minute. A live test wrote the list from outside while other sessions
rewrote the file seven times in five minutes, and the change survived: Claude
Code merges keys it does not own. A running session never picked up an outside
change, so `--restart` writes while the session is stopped, and `--in` refuses a
directory where a session runs. Writing the bytes Claude Code would write keeps
the file's diff to the one list, which a user who keeps the file in a dotfiles
repository sees.

Weighed: decoding and encoding the whole file, which reorders keys and changes
escapes; a lock file next to it, which Claude Code is not known to take; and
writing while the session runs, which the session would ignore until it
restarts.

## 39. A profile launches through `allowedMcpServers`, matched by URL or command

`claudes agents --profile NAME` runs `claude agents --settings` with an
`allowedMcpServers` setting built from the profile, in a file only the user can
read ([Using a profile](guide.md#using-a-profile)). Each server claudes finds
goes in by its `serverUrl`, or by its `serverCommand` with
`${CLAUDE_PLUGIN_ROOT}` filled in, and any other by its `serverName`.
`claudes mcp use` applies a profile to a session that already runs, through the
same path as `mcp on` and `mcp off`.

Why: an allowlist at launch gives agent view and every session it starts exactly
the profile's servers, with no change to any directory's settings, and a
respawned session keeps it. Claude Code matches a plugin's server only by its
URL or command. A live test showed more: once the list holds one
`serverCommand`, a stdio server named only by `serverName` is left out, so
claudes uses commands and URLs wherever it can. A file keeps the servers' URLs
and commands, which can carry tokens, out of the process list that
`--settings JSON` would put them in.

Weighed: a `deniedMcpServers` list of everything else, which lets in every
server added later; names alone, which drop plugin servers and, beside one
command, every stdio server; and turning servers off in each directory before
launch, which changes settings other sessions share.
