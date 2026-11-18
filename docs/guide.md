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

### Listing sessions

`claudes ls` lists the running sessions, and `-a` adds finished background ones.
Its `AGENTS` column shows the agents a session runs: `lead+2` when it leads an
agent team with two teammates at work, `3` when three of its subagents run now,
and `lead+2, 3` for both. `-` means neither. `--wide` adds four columns:

- `SHELLS`: the background shells it runs now.
- `TOKENS`: agent view's running total of the tokens the session has used, such
  as `1.4M`. It keeps growing over the session's life, so it is not the size of
  the context.
- `PRS`: the pull requests the session opened.
- `DOING`: agent view's one-line status of what the session is doing. It comes
  last, after `NAME`, and is cut to fit the terminal, or to 50 characters when
  the output goes elsewhere, so it never wraps.

A long `DIR` is shortened to fit too, as
[Directories in tables](reference.md#directories-in-tables) describes, and
`output.dirs` picks how.

The subagent count and the `--wide` columns come from the job's `state.json` in
`~/.claude/jobs`, which is not a stable Claude Code interface. Interactive
sessions have no job, so they show `-`, as does a field Claude Code stops
writing. A session with no process runs no subagents or shells, whatever its
`state.json` lists.

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
| `-u`, `--outdated`  | `restart`                          | Only sessions on an older Claude Code                             |
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
   sessions; add `--pinned` to take them all. Named sessions replace the bulk
   selection, so to include one pinned session, name it and every other session
   you want.
2. **Mid-turn work isn't dropped silently.** A session that is busy, waiting on
   a question, or running a shell task comes back idle after a restart. `--ping`
   stops it and resumes it with a prompt instead, and `--wait` waits for busy
   ones first.
3. **Nothing is stopped that can't come back.** Before stopping a session it
   will resume, claudes parks it, waits for its process to end, then resumes it.
   If the resume fails, the session stays parked for `claudes resume`. `pause`,
   `ping`, and a restart that resumes a session leave it running when its
   directory is gone, since it could not come back. `stop` stops it anyway.
4. **A resumed session keeps its settings.** Claude Code wakes a background
   session with the options it started with, such as `--model` and
   `--permission-mode`, so a ping, a restart, or `resume` keeps them.
5. **Restarts are checked.** After a restart, claudes checks that each respawned
   session is running again.
6. **Its own session goes last.** The session running claudes is handled after
   everything else is printed, because stopping it ends the command.
7. **Agent teams stay intact.** Stopping a session that leads an agent team also
   stops its teammates, and in-process teammates cannot be resumed. So bulk
   `restart` and `pause` leave such a lead alone. Named, with every other
   session you want, it goes ahead: the plan, `stop`, and `ping` name the
   teammates that stop with it, and a pinged lead can start them again. Check
   the `AGENTS` column of `claudes ls` before you `stop` or `ping` one.
8. **Ctrl-C is graceful.** The first one finishes the session in hand, leaves
   the rest untouched, and lists them. A second one ends claudes at once.
9. **Unfamiliar data stops it.** See
   [Compatibility](reference.md#compatibility).
10. **One claudes changes sessions at a time.** Commands that stop, start,
    remove, or pin sessions take a lock, `run.lock` in claudes' state directory,
    so two runs cannot stop the same session twice. A second one stops before it
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
`--in PATH` (`-i`) for sessions run in a directory and below it, and
`--since 7d` (`-l`, for "last") for recent ones. `session where --one` fails
unless exactly one session matches, for scripts. `session resume` refuses a
session that is still running, since resuming it would start a copy; `--bg`
(`-b`) resumes in the background. `session search` finds a session by what was
said in it; see [Searching past sessions](#searching-past-sessions).

`session ls` shows the space each session takes on disk: its transcript, its
subagent transcripts and tool results, older copies `session mv` left, and
Claude Code's backups of the files it edited, under `~/.claude/file-history`.
`--sort size` lists the biggest first, and `session show` breaks one session's
size down. `session show` also reads the whole transcript to count the prompts,
slash commands, tool calls, and file edits, and says where the name came from:
`/rename`, agent view, or Claude Code's own title. `session du` (or `df`) adds
it all up by project directory, biggest first, and counts leftovers: subagent
and file-history directories whose transcript is gone. `--top 10` (`-t 10`) also
lists the 10 sessions that take the most space, with the share their sidecar
directory of tool results and subagent transcripts takes.

`session gc` removes past sessions with everything Claude Code keeps for each:
transcript, subagent transcripts and tool results, older copies, file history,
session-env directory, and debug log. They go to the system trash (the macOS
Trash, the Windows Recycle Bin, or the freedesktop.org trash on Linux), so a
mistake can be undone; `--purge` deletes them for good. Windows deletes an item
too big for the Recycle Bin, or on a drive without one, for good, without
asking. Without flags it removes:

1. Empty sessions, which never got a prompt.
2. Older copies `session mv` left, keeping the newest.
3. Leftovers: subagent and file-history directories whose transcript is gone.

A session is empty only when its transcript holds nothing but records claudes
knows Claude Code writes about a session, such as its mode and title. A record
type claudes does not know, a record with a message, or a line that does not
parse keeps the session, since Claude Code adds and renames record types.

`--empty`, `--copies`, `--leftovers`, `--trivial`, `--gone` (the session's
directory no longer exists), and `--older-than 90d` pick exactly those kinds
instead. A trivial session changed no file and made no tool call, and either
only ran slash commands, such as `/login`, or got one prompt, such as a quick
health check. A session named with `/rename`, one an agent team's lead started,
and one with file history are never trivial. To tell, claudes reads each
transcript up to 2 MB in full; a bigger one is never trivial, and neither is one
with a record type claudes does not know or a line that does not parse. Targets
remove exactly the sessions named. `--dry-run` shows the plan and what it frees.

`--sidecars` keeps every session and trims its sidecar directory instead, which
is often most of a big session (`session du --top 10` shows how much). It takes
the big command outputs Claude Code saved under `tool-results`. The transcript
keeps a preview of each, so the session resumes with everything the model saw,
but cannot read a saved output again. With `--older-than 30d`, it also takes the
subagent transcripts of sessions idle that long. Such a session still resumes,
but cannot continue a subagent it ran, and `claudes logs X/TEAMMATE` has nothing
to show. With `--sidecars`, `--older-than` removes no whole session.

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
transcripts and tool results, into the project directory Claude Code uses for
`DIR`, which must already exist. Use it after cloning a repository mid-session,
or when the work outgrew its directory. It keeps the original and prints the
command that removes it, then the command that resumes the copy.

It refuses:

1. A running session, except the one running claudes: copy that one, then exit
   it and run the printed command.
2. When it cannot tell which sessions run, unless you pass `--force`.
3. When `DIR` already holds a copy, which may have newer work, or a subagent
   directory of the session.
4. When recent sessions live in project directories claudes would not name that
   way, so it would copy to the wrong one. `--force` goes ahead.
5. When `claude agents` lists something claudes does not recognize, before it
   removes a job (see [Compatibility](reference.md#compatibility)). `--force`
   goes ahead, and also lets `session resume --bg` start one.

It copies into a temporary directory first and renames the copy into place, so a
copy that fails partway leaves nothing behind, and running `mv` again starts
over.

`--resume` (`-r`) resumes the copy at once, and `--rm` deletes the original once
every file of the copy matches it byte for byte. `--rm` refuses the session
running claudes, which writes to its transcript until it exits.

For a stopped background session, `mv` also removes its job with `claude rm`, as
`gc` does, and keeps the options Claude Code saved for the job, such as
`--model` and `--permission-mode`. A job wakes in the directory it started in,
wherever `claude --bg --resume` runs from, so the old job would resume the
session in the old directory. The next `session resume --bg` starts a new job in
`DIR` with the saved options. `mv` keeps the job, and says why, when:

1. The job is not stopped, such as one that started again after claudes listed
   it.
2. The session is paused, since `claudes resume` wakes it where it was paused.
3. claudes cannot read the job's saved options, such as a `state.json` without
   its `respawnFlags`.

Until such a job is gone, `session resume --bg` refuses the session and prints
the `claudes gc` command that removes the job.

A copy's transcript still names the old directory until it is first resumed from
the new one, so claudes records each move in `moves.json`, with the options of
any job it removed. `session where`, `cd`, and `session resume` then use the new
directory right away.

`mv` copies rather than moving because a copy cannot lose the original if
anything goes wrong, and because Claude Code's own record of a moved session, a
`relocated` entry in the transcript, is not documented. claudes reads those
entries, so a session Claude Code moved resumes from its new directory.

### Searching past sessions

`session search QUERY...` (or `grep`) finds sessions by what was said in them,
best first. A session must match every word. Each word matches in any case at
the start of a word, so `lock` matches `locks`, `locking`, `fileLock`, and
`/src/lock.go`, but not `block`. A word starts after a character that is not a
letter or a digit, such as a space, a slash, or an underscore, and at an
upper-case letter after a lower-case one. Chinese, Japanese, Korean, Thai, and
the other scripts written without spaces between words start a word at every
character, so `東京` matches in `私は東京に行きました`. A word that starts with
punctuation, such as `.go`, matches anywhere. Quote words to match them as a
phrase. It reads every transcript in full, and looks in four places, which rank
in this order:

1. The session's name.
2. The prompts you typed.
3. Claude's replies.
4. Claude's tool calls: the commands it ran, and the paths and patterns it used,
   but not the text it wrote into files.

It skips slash commands, what the tools returned, which `--tools` adds, and the
text Claude Code adds itself, such as reminders and compaction summaries. That
text repeats across sessions and would match everywhere.

A session ranks by its best record: one prompt, reply, or tool call. A record
that holds the words next to each other, in order, ranks first, then one that
holds every word, then one that holds some. So a prompt about a `lock race`
outranks a session that says `lock` in one prompt and `race` in another. Within
that, the place counts: both words in a reply still beat them scattered over
prompts. More matches and recent activity rank a session a little higher.

Each line shows the best match in context, from the best record, and `-o json`
gives each session's best records. It shows 20 sessions; `--top N` (`-t`)
changes that, and `--top 0` shows them all. Resume one with `session resume ID`.
These flags narrow the search:

1. `--who you` or `--who claude` matches QUERY only in your prompts or only in
   Claude's replies.
2. `--ran TEXT` keeps sessions where the Bash tool ran a command containing
   TEXT, and `--touched PATH` those where Claude edited or wrote a file whose
   path contains PATH. Neither needs QUERY.
3. `-e PATTERN` (`--regexp`) matches a regular expression, alone or as well as
   QUERY, in [Go's RE2 syntax](https://pkg.go.dev/regexp/syntax): Perl's,
   without lookaround or backreferences. It matches anywhere, not only at the
   start of a word, and is case-sensitive unless it starts with `(?i)`.
4. `--in PATH` (`-i`), `--since AGE` (`-l`), and `--branch NAME` narrow the
   sessions searched, as in `session ls`.

Search masks secrets in what it prints, in text and in `-o json`: the session's
name and every snippet. Each secret prints as `[redacted]`:

1. Provider keys, such as `sk-…`, `sk-ant-…`, `ghp_…`, `gho_…`, `github_pat_…`,
   `xoxb-…`, `AKIA…`, `AIza…`, and `glpat-…`.
2. The credential after `Authorization: Bearer`, `Basic`, or `token`.
3. The value of a name that ends in `KEY`, `TOKEN`, `SECRET`, `PASSWORD`, or
   `PASSWD`, as in `export GITHUB_TOKEN=…`, `--password=…`, or `"api_key": "…"`.
   The name stays, so it prints `GITHUB_TOKEN=[redacted]`. A value shorter than
   six characters stays, and so does one that names a secret, such as
   `$GITHUB_TOKEN` or `<token>`.
4. The user and password in a URL, as in `https://user:pass@host`.
5. The body of a private key block.

Matching still reads the text as written, so a word inside a masked value finds
its session. Masking goes by shape, so a secret of a shape it does not know
still prints. `--no-redact` prints everything as it was written.

## Agent teams

With Claude Code's agent teams on, a session can lead teammates that work in the
same process. `claude agents` lists only the lead; claudes shows the rest.
`claudes t` and `claudes teams` are short for `claudes team`.

```console
$ claudes team ls
ID        TEAMMATES       TASKS                      DIR       LEAD
62d022d3  writer, tester  1/2 done, working: tester  ~/src/rev  teams-lead
```

- `ls` marks a lead with its teammate count in the `AGENTS` column, such as
  `lead+2`, and `-o json` lists them under `teammates`.
- `claudes team ls` lists the teams with teammates at work, their task progress,
  and who works on the open tasks. `-a` adds every team Claude Code keeps,
  including those whose lead no longer runs.
- `claudes logs LEAD/NAME` prints a teammate's last events from its own
  transcript, and `-f` follows it. In a lead's log, a message from a teammate
  prints as one line, such as `team  tester: idle: Task #2 is complete`.
- Bulk `restart` and `pause` leave a lead with teammates at work alone, since
  its teammates would stop with it and cannot be resumed (see Safety).
- `claudes gc --teams` removes the team directories and task lists Claude Code
  leaves behind: those of sessions that no longer run, are not parked or pinned,
  and have not changed for longer than `--older-than`, 7 days by default.
  `--dry-run` shows them first.

## MCP servers

`claudes mcp ls` lists the MCP servers Claude Code gives a session, and whether
each one is on. Name a session, past or running, to list the servers of its
directory, or pass `--in DIR` (`-i`). With neither, it lists the current
directory's.

```console
$ claudes mcp ls api
MCP servers in ~/src/api:
NAME                SCOPE    STATE    KIND   PROFILES
browser             user     on       stdio  safe, web
plugin:docs:search  plugin   on       http   safe
repo-tool           project  pending  stdio  -
issue-tracker       user     off      http   web
```

1. `SCOPE` says where the server comes from: `user` is `mcpServers` in
   `~/.claude.json`, `local` is that directory's entry there, `project` is a
   `.mcp.json` in the directory or one above it, `plugin` is an enabled plugin,
   and `managed` is an administrator's `managed-mcp.json`. When two scopes
   define one name, claudes lists the one Claude Code uses: local, then project,
   then user, then plugin. A `managed-mcp.json` replaces all the others.
2. `STATE` is `on`, `off` when `/mcp` turned the server off in that directory,
   or `pending` for a project server nobody has approved or rejected yet. Claude
   Code saves that answer in `~/.claude.json` or a settings file, as
   `enableAllProjectMcpServers`, `enabledMcpjsonServers`, or
   `disabledMcpjsonServers`. A rejected server is off.
3. `KIND` is the transport, such as `stdio`, `http`, or `sse`.
4. Servers that are on come first, then pending ones, then those that are off,
   each by name, so what a session loads reads first.

claudes never prints a server's command, arguments, environment, headers, or
URL, because they can carry tokens.

The files claudes reads are not a stable Claude Code interface. A field it
cannot read shows `-`, and a file it cannot read gets a warning while the rest
of the list still prints. claudes does not show claude.ai connectors or servers
passed with `--mcp-config`, and does not apply the `allowedMcpServers` and
`deniedMcpServers` settings.

### MCP profiles

A profile names the servers that stay on for one kind of work. Profiles live
under `mcp.profiles` in [config.yaml](reference.md#configuration), and
`claudes config edit` changes them.

```yaml
mcp:
  profiles:
    safe: [browser, "plugin:docs:search"]
    web: [browser, issue-tracker]
```

`claudes mcp profiles` lists them, and warns about a name that matches no server
claudes can find: one of the current directory's, any directory's in
`~/.claude.json`, or an installed plugin's. `claudes mcp ls --profile safe` adds
a `PROFILE` column with each server's state under that profile, next to the
current one. The `PROFILES` column of `mcp ls` names the profiles that keep each
server on.

`mcp ls` and `mcp profiles` only read. Turning servers on and off, and starting
a session with a profile, come in a later release.

## Following a session

`claudes logs NAME` prints a background session's recent screen, as
`claude logs` does. `-f` follows the session's transcript instead: each prompt,
reply, tool call, and turn's end prints as one line as it happens, until Ctrl-C,
and the session keeps running. `--tail N` (`-t N`) starts with the last N
events, 10 by default with `-f`, or prints them and exits without `-f`. Both
work for interactive and finished sessions too.

```console
$ claudes logs -f api --tail 2
20:41:45 you    Run the tests.
20:41:47 tool   Bash: go test ./...
20:41:58 claude All 214 tests pass.
20:41:59 turn   done in 14s
```
