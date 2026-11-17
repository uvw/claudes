# Using claudes

How claudes names and handles sessions, what each kind of command does, and how
it keeps work from being lost. The [README](../README.md#commands) lists every
command; `claudes COMMAND --help` describes each one's flags.

## Words claudes uses

- **Background session (`bg`):** one started with `claude --bg`. It has a job
  id, and agent view can stop, respawn, and remove it.
- **Interactive session (`tty`):** Claude Code running in a terminal. claudes
  leaves these alone unless you ask, since stopping one ends that terminal's
  session.
- **Mid-turn:** a session that is `busy`, `waiting` on a question, running a
  `shell` task, or leading an agent team with teammates at work. Stopping it
  drops that work, so claudes resumes it with a prompt instead (`--ping`).
- **Pin:** agent view's Ctrl+T. Bulk commands skip pinned sessions.
- **Park:** claudes' record of a session it stopped and means to bring back, in
  `parked.json`. `claudes resume` reads it.
- **Target:** how you name a session: its name, job id, session id, or short id
  (the first eight characters of the session id). See
  [Naming sessions](#naming-sessions).

## Commands

`claudes` with no command, or with arguments that are not a claudes command,
runs `claude agents` with those arguments. So a mistyped command, such as
`claudes lst`, opens agent view rather than failing. `claudes -- ARGS` always
passes through, even when ARGS starts with a claudes command name, so a script
stays correct if claudes adds a command of the same name. In PowerShell with
`claudes init` loaded, quote it as `claudes '--' ARGS`: a PowerShell function
never receives a bare `--`. `claudes COMMAND --help` describes each command and
its flags.

A ping cuts off a turn in progress, and the session takes MSG as its next
prompt. With no MSG, `ping` sends "Pick up where you left off.".
`restart --ping` and `resume --ping` send "Claude Code was restarted while you
were working. Pick up where you left off." to the sessions that were mid-turn;
`ping.text` in [Configuration](reference.md#configuration) or `--ping-text`
changes both. `gc` removes through `claude rm`, which keeps each conversation,
so `session resume` still finds it, and deletes the session's worktree only when
that is safe.

### Common flags

| Flag                | Commands                           | Effect                                                            |
| ------------------- | ---------------------------------- | ----------------------------------------------------------------- |
| `-n`, `--dry-run`   | `restart`, `pause`, `resume`, `gc` | Show the plan and change nothing                                  |
| `-c`, `--confirm`   | `restart`, `pause`, `resume`       | Ask y/N before changing anything                                  |
| `-s`, `--skip NAME` | `restart`, `pause`, `resume`       | Leave out a session, matched like a target                        |
| `-w`, `--wait`      | `restart`, `pause`                 | Wait for busy sessions to go idle first                           |
| `--timeout AGE`     | `restart`, `pause`                 | With `--wait`, give up after this long; 30m by default            |
| `-p`, `--ping`      | `restart`, `resume`                | Prompt sessions that were mid-turn so they carry on               |
| `--ping-text TEXT`  | `restart`, `resume`                | The prompt to send; implies `--ping`                              |
| `--interactive`     | `restart`, `pause`                 | Also stop interactive sessions, and print their resume commands   |
| `--interactive-bg`  | `restart`, `resume`                | Resume interactive sessions in the background, not print commands |
| `--outdated`        | `restart`                          | Only sessions on an older Claude Code                             |
| `--pinned`          | `restart`, `pause`, `gc`           | Without targets, take pinned sessions too                         |
| `--forget`          | `resume`                           | Drop the named sessions from the park list                        |
| `--older-than AGE`  | `gc`                               | Without targets, only sessions older than AGE; 7d                 |
| `--force`           | commands that change sessions      | Act although a compatibility check failed                         |
| `-o`, `--output`    | every command that lists or acts   | Print `text`, `json`, or `yaml`                                   |
| `-v`, `--verbose`   | every command                      | Log each `claude` call and step to stderr                         |

`--older-than` and `--since` take days, such as `7d`, as well as `12h` or `30m`.
`--timeout` and `wait.timeout` take `30m` or `1h30m`, not days.

## Naming sessions

A target names a session by its name, job id, session id, or short id.

Commands that change something need each target to name one session. An exact
match wins, then the start of a name or id, then part of a name; the last two
ignore case. The start of an id needs at least four characters. A target that
matches several sessions in the same step is an error that lists them. A target
that names a session of the wrong kind or state, such as a stopped one for
`stop`, is refused with exit 4.

Commands that only look, `ls` and `where` and their `session` versions, list
every session a target fits: `ls plan` lists every session with `plan` in its
name. A target with `*`, `?`, or `[` is a pattern over whole names and ids,
ignoring case, so `ls 'plan*'` lists only names that start with `plan`. `ls`,
`session ls`, and `team ls` show what their targets match and exit 0, and note a
target that matches nothing on stderr, while `where` fails on one with exit 2,
so `cd "$(claudes where NAME)"` notices a typo.

Sessions reuse names over time, so for `session show`, `resume`, `mv`, and `cd`
a name picks the newest session that has it.

## Safety

claudes is built to restart and stop sessions without losing work:

1. **Pins protect sessions.** Pins are agent view's own (Ctrl+T, stored in
   `~/.claude/jobs/pins.json`). Bulk `restart`, `pause`, and `gc` skip pinned
   sessions; name one to include it, or add `--pinned` to take them all.
2. **Mid-turn work isn't dropped silently.** A session that is busy, waiting on
   a question, or running a shell task comes back idle after a restart. `--ping`
   stops it and resumes it with a prompt instead, and `--wait` waits for busy
   ones first.
3. **Nothing is stopped that can't come back.** Before stopping a session it
   will resume, claudes parks it, waits for its process to end, then resumes it.
   If the resume fails, the session stays parked for `claudes resume`. `pause`,
   `ping`, and a restart that resumes a session leave it running when its folder
   is gone, since it could not come back. `stop` stops it anyway.
4. **A resumed session keeps its settings.** Claude Code wakes a background
   session with the options it started with, such as `--model` and
   `--permission-mode`, so a ping, a restart, or `resume` keeps them.
5. **Restarts are checked.** After a restart, claudes checks that each respawned
   session is running again.
6. **Its own session goes last.** The session running claudes is handled after
   everything else is printed, because stopping it ends the command.
7. **Agent teams stay intact.** Stopping a session that leads an agent team also
   stops its teammates, and in-process teammates cannot be resumed. So bulk
   `restart` and `pause` leave such a lead alone. Named, it goes ahead: the
   plan, `stop`, and `ping` name the teammates that stop with it, and a pinged
   lead can start them again. Check the `TEAM` column of `claudes ls` before you
   `stop` or `ping` one.
8. **Ctrl-C is graceful.** The first one finishes the session in hand, leaves
   the rest untouched, and lists them. A second one ends claudes at once.
9. **Unfamiliar data stops it.** See
   [Compatibility](reference.md#compatibility).
10. **One claudes changes sessions at a time.** Commands that stop, start,
    remove, or pin sessions take a lock, `run.lock` in claudes' state folder, so
    two runs cannot stop the same session twice. A second one stops before it
    changes anything; `--dry-run` and the commands that only look never wait. A
    claudes that crashed frees the lock within 30 seconds.

### Interactive sessions

Bulk commands leave interactive sessions (Claude Code in a terminal) alone and
print the command that resumes each one, in POSIX shell syntax on macOS and
Linux and PowerShell syntax on Windows. `--interactive` stops them, parks them,
and prints their resume commands, and `--interactive-bg` resumes them in the
background instead. `ping` and `pin` take background sessions only.

`stop` and `--interactive` end an interactive session with SIGTERM, then SIGKILL
after ten seconds, and then do the same to any process it started that outlived
it, such as a shell or an MCP server. Windows has no SIGTERM for console
programs, so there the session and every process it started are killed. A parked
interactive session stays parked until it is seen running, so `claudes resume`
prints its command until then; `claudes resume --forget NAME` drops it.

## Past sessions

`claudes session` reads the transcripts Claude Code keeps under
`~/.claude/projects`, so it finds every session, not only those agent view
lists. `claudes s` and `claudes sessions` are short for it. `session ls` takes
`--in PATH` for sessions run in a folder and below it, and `--since 7d` for
recent ones. `session where --one` fails unless exactly one session matches, for
scripts. `session resume` refuses a session that is still running, since
resuming it would start a copy; `--bg` resumes in the background.

`session ls` shows the space each session takes on disk: its transcript, its
subagent transcripts and tool results, older copies `session mv` left, and
Claude Code's backups of the files it edited, under `~/.claude/file-history`.
`--sort size` lists the biggest first, and `session show` breaks one session's
size down. `session show` also reads the whole transcript to count the prompts,
slash commands, tool calls, and file edits, and says where the name came from:
`/rename`, agent view, or Claude Code's own title. `session du` (or `df`) adds
it all up by project folder, biggest first, and counts leftovers: subagent and
file-history folders whose transcript is gone. `--top 10` also lists the 10
sessions that take the most space, with the share their sidecar folder of tool
results and subagent transcripts takes.

`session gc` removes past sessions with everything Claude Code keeps for each:
transcript, subagent transcripts and tool results, older copies, file history,
session-env folder, and debug log. They go to the system trash (the macOS Trash,
the Windows Recycle Bin, or the freedesktop.org trash on Linux), so a mistake
can be undone; `--purge` deletes them for good. Without flags it removes:

1. Empty sessions, which never got a prompt.
2. Older copies `session mv` left, keeping the newest.
3. Leftovers: subagent and file-history folders whose transcript is gone.

`--empty`, `--copies`, `--leftovers`, `--trivial`, `--gone` (the session's
folder no longer exists), and `--older-than 90d` pick exactly those kinds
instead. A trivial session changed no file and made no tool call, and either
only ran slash commands, such as `/login`, or got one prompt, such as a quick
health check. A session named with `/rename`, one an agent team's lead started,
and one with file history are never trivial. To tell, claudes reads each
transcript up to 2 MB in full; a bigger one is never trivial. Targets remove
exactly the sessions named. `--dry-run` shows the plan and what it frees.

`session gc` never removes a running session, one agent view still lists
(`claudes gc` removes those from the list first), a parked one, or the session
running claudes. Without targets, it also leaves alone anything that changed in
the last day. It checks again which sessions run just before removing, and only
removes paths under `~/.claude` named by the session's id.

`session triage` sorts past sessions into three groups and changes nothing:

1. **Remove:** what `session gc` takes without flags, and trivial sessions. It
   prints the `session gc` command that removes them.
2. **Review:** sessions with one prompt, or only slash commands, that changed no
   file and made at most 20 tool calls, and that were not named with `/rename`.
   Whether to keep one takes reading its prompt: a commit message or a quick
   lookup can go, while findings or a decision should stay. `-o json` gives each
   one's first and last prompt, and the [agent skill](install.md#agent-skill)
   tells an agent how to judge them. Remove the ones you pick with
   `session gc ID...`.
3. **Keep:** everything else, counted by reason, such as more than one prompt,
   changed files, or running.

`session mv` (or `move`) copies a session's transcript, with its subagent
transcripts and tool results, into the project folder Claude Code uses for
`FOLDER`, which must already exist. Use it after cloning a repository
mid-session, or when the work outgrew its folder. It keeps the original and
prints the command that removes it, then the command that resumes the copy.

It refuses:

1. A running session, except the one running claudes: copy that one, then exit
   it and run the printed command.
2. When it cannot tell which sessions run, unless you pass `--force`.
3. When `FOLDER` already holds a copy, which may have newer work.
4. When recent sessions live in project folders claudes would not name that way,
   so it would copy to the wrong one. `--force` goes ahead.

`--resume` resumes the copy at once, and `--rm` deletes the original once every
file of the copy matches it byte for byte. `--rm` refuses the session running
claudes, which writes to its transcript until it exits.

A copy's transcript still names the old folder until it is first resumed from
the new one, so claudes records each move in `moves.json`. `session where`,
`cd`, and `session resume` then use the new folder right away.

`mv` copies rather than moving because a copy cannot lose the original if
anything goes wrong, and because Claude Code's own record of a moved session, a
`relocated` entry in the transcript, is not documented. claudes reads those
entries, so a session Claude Code moved resumes from its new folder.

## Agent teams

With Claude Code's agent teams on, a session can lead teammates that work in the
same process. `claude agents` lists only the lead; claudes shows the rest.
`claudes t` and `claudes teams` are short for `claudes team`.

```console
$ claudes team ls
LEAD        ID        TEAMMATES       TASKS                      CWD
teams-lead  62d022d3  writer, tester  1/2 done, working: tester  ~/src/rev
```

- `ls` marks a lead with its teammate count, such as `lead+2`, and `-o json`
  lists them under `teammates`.
- `claudes team ls` lists the teams with teammates at work, their task progress,
  and who works on the open tasks. `-a` adds every team Claude Code keeps,
  including those whose lead no longer runs.
- `claudes logs LEAD/NAME` prints a teammate's last events from its own
  transcript, and `-f` follows it. In a lead's log, a message from a teammate
  prints as one line, such as `team  tester: idle: Task #2 is complete`.
- Bulk `restart` and `pause` leave a lead with teammates at work alone, since
  its teammates would stop with it and cannot be resumed (see Safety).
- `claudes gc --teams` removes the team folders and task lists Claude Code
  leaves behind: those of sessions that no longer run, are not parked or pinned,
  and have not changed for longer than `--older-than`, 7 days by default.
  `--dry-run` shows them first.

## Following a session

`claudes logs NAME` prints a background session's recent screen, as
`claude logs` does. `-f` follows the session's transcript instead: each prompt,
reply, tool call, and turn's end prints as one line as it happens, until Ctrl-C,
and the session keeps running. `--tail N` starts with the last N events, 10 by
default with `-f`, or prints them and exits without `-f`. Both work for
interactive and finished sessions too.

```console
$ claudes logs -f api --tail 2
20:41:45 you    Run the tests.
20:41:47 tool   Bash: go test ./...
20:41:58 claude All 214 tests pass.
20:41:59 turn   done in 14s
```
