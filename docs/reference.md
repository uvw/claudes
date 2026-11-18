# Reference

Output formats, exit codes, what to do when something goes wrong, configuration,
compatibility checks, logging, and the files claudes keeps.

## Output

Pick a format with `-o`:

| Commands                                                    | Formats          | Default                  |
| ----------------------------------------------------------- | ---------------- | ------------------------ |
| `ls`, `where`, `team ls`                                    | text, json, yaml | `output.format`, or text |
| `session ls`, `show`, `where`, `du`, `search`, and `triage` | text, json, yaml | `output.format`, or text |
| `doctor`, `doctor env`                                      | text, json, yaml | `output.format`, or text |
| `mcp ls`, `mcp profiles`                                    | text, json, yaml | `output.format`, or text |
| `restart`, `pause`, `resume`, `ping`, and `stop`            | text, json, yaml | text                     |
| `gc`, `pin`, `unpin`, `session gc`                          | text, json, yaml | text                     |
| `logs`                                                      | text, json       | text                     |

A few cases differ:

1. `ls --json` is short for `ls -o json`.
2. When `where` matches one session, text prints only its directory, so
   `cd "$(claudes where NAME)"` works. Add `-o text` when `output.format` sets
   another format; `claudes init` does that itself. With several, it prints a
   table of id, directory, and name, as every table does.
3. `logs -o json` prints the transcript records themselves, one per line.

Every text table, and every plan a command prints before it acts, starts with
the id and ends with the directory and the name, followed by a note when there
is one, or by `DOING` in `ls --wide`. Names longer than 40 characters are cut,
with `…`; JSON, YAML, and `session show` keep them whole. The `mcp` tables list
servers and profiles rather than sessions, so they start with the name, and end
with the profiles.

### Directories in tables

A table shows a directory with `~` for the home directory, and shortens one that
does not fit its column, so a deep path does not wrap every row. This covers
every text table and plan, the table `where` prints when several sessions match,
and the heading of `mcp ls`. JSON, YAML, `session show`, `doctor env`, and the
bare directory `where` prints for one session keep the whole path, since scripts
read them.

On a terminal, a directory gets the width the other columns leave on the line,
and at least 20 columns. A wide window shows the whole path, and a narrow one
shortens it. Elsewhere, such as in a pipe, a directory takes at most 40 columns.
A path that fits is never shortened.

When `DOING` in `ls --wide` or `MATCH` in `session search` ends the line, the
directory is fitted first, as though that column took 10 columns. The column
then gets what is left, and at least 10. Off a terminal, `DOING` is cut to 50
characters and `MATCH` stays whole.

`output.dirs` in `config.yaml` picks how a directory is shortened. Each example
fits `~/code/acme/monorepo/services/billing/internal/ledger/testdata/v2` into 30
columns:

1. `middle`, the default, gives `~/code/…/ledger/testdata/v2`, and
   `~/code/…/internal/ledger/testdata/v2` in 40 columns. It puts `…` in place of
   the directories in the middle and keeps whole names: the last directory, then
   `~` or `/` and the first directory after it, then as many directories before
   the last as fit, then a second directory after `~` or `/`. When the first
   directory does not fit beside the last, it goes before the last is cut.
2. `fish` gives `~/c/a/m/s/b/i/l/testdata/v2`. It shortens each directory but
   the last to its first character, or its first two when it starts with a dot,
   such as `.c` for `.claude`. It goes from the left and stops once the path
   fits, so the directories nearest the last stay whole when there is room.
3. `full` never shortens.

With `middle` or `fish`, a last directory too long to fit on its own is cut in
its middle too, such as `~/…/a-directory-name-f…-in-forty-columns` in 40
columns. `fish` falls back to `middle` when even its shortest form does not fit.

### Fields

JSON and YAML use the camelCase field names of `claude agents --json`. Every key
is always present, and an empty list is `[]`.

#### Running sessions

`ls` and `where` print a list of:

| Field                     | Meaning                                                  |
| ------------------------- | -------------------------------------------------------- |
| `id`                      | The short job id; empty for an interactive session       |
| `sessionId`               | The full session id                                      |
| `name`, `cwd`             | Its name and directory                                   |
| `kind`, `state`, `status` | As `claude agents` reports them                          |
| `pid`, `startedAt`        | Its process, and when it started, in Unix milliseconds   |
| `pinned`                  | Whether it is pinned in agent view                       |
| `version`                 | The Claude Code it runs                                  |
| `outdated`                | Whether that is older than the installed Claude Code     |
| `teammates`               | The names of its agent team's members, when it leads one |
| `subagents`, `shells`     | How many subagents and background shells it runs now     |
| `tokens`                  | Agent view's running total of the tokens it has used     |
| `prs`                     | How many pull requests it opened                         |
| `doing`                   | Agent view's one-line status of what it is doing         |

`subagents`, `shells`, `tokens`, `prs`, and `doing` come from the job's
`state.json`, and are `null` when claudes cannot tell, as for an interactive
session. `subagents` leaves out teammates, which `teammates` lists. `tokens`
keeps growing over the session's life, so it is not the size of the context.

#### Past sessions

`session ls` prints a list of these, and `session show` prints one:

| Field                   | Meaning                                                     |
| ----------------------- | ----------------------------------------------------------- |
| `id`, `sessionId`       | The short and the full session id                           |
| `name`                  | Its name, or empty                                          |
| `nameSource`            | Where the name came from: `custom`, `agent`, `ai`, or empty |
| `cwd`, `branch`         | The directory and git branch it ran in                      |
| `started`, `lastActive` | When it started, and when its transcript last changed       |
| `lastPrompt`            | The last prompt                                             |
| `version`               | The Claude Code that wrote the transcript                   |
| `path`                  | The transcript                                              |
| `copies`                | The older copies `session mv` left                          |
| `size`                  | The transcript's size in bytes                              |
| `empty`                 | Whether it never got a prompt                               |
| `running`               | Whether it runs now                                         |
| `disk`                  | The bytes it takes on disk, broken down                     |
| `activity`              | What it did; null in `session ls`                           |

`nameSource` is `custom` for a name set with `/rename`, `agent` for agent view's
name, and `ai` for Claude Code's own title. `disk` has `transcript`, `sidecar`,
`copies`, `fileHistory`, and `total`. `activity` comes from reading the whole
transcript, and has `prompts`, `commands` (slash and `!` commands), `toolCalls`,
`edits`, `firstPrompt`, and `teammate`, which is true when an agent team's lead
started the session.

#### Disk use

`session du` prints one object:

| Field       | Meaning                                                       |
| ----------- | ------------------------------------------------------------- |
| `dirs`      | The project directories, biggest first                        |
| `top`       | The biggest sessions, with `--top`                            |
| `leftovers` | Sidecar and file-history directories whose transcript is gone |
| `total`     | The bytes all of it takes, leftovers included                 |

Each entry in `dirs` has `dir`, `project`, `sessions`, and `disk`. Each session
in `top` has `id`, `sessionId`, `name`, `cwd`, `running`, and `disk`. Each
leftover has `path`, `sessionId`, `kind`, `size`, and `modified`.

#### Search

`session search` prints a list of the sessions that match, best first:

| Field             | Meaning                                         |
| ----------------- | ----------------------------------------------- |
| `id`, `sessionId` | The short and the full session id               |
| `name`, `cwd`     | Its name, or empty, and the directory it ran in |
| `score`           | How well it matches; higher is better           |
| `lastActive`      | When its transcript last changed                |
| `matches`         | The records that matched best, the best first   |

Each match has `where`, which is `name`, `you` for your prompts, `claude` for
Claude's replies, `tool` for its tool calls, or `output` for what the tools
returned, and `snippet`, the match in context on one line. Each place gives its
three best records at most. The name and each snippet have their secrets masked
as `[redacted]`, unless `--no-redact` is given;
[Searching past sessions](guide.md#searching-past-sessions) lists what is
masked.

#### Triage

`session triage` prints `remove`, `review`, and `keep`, each a list of:

| Field                     | Meaning                                            |
| ------------------------- | -------------------------------------------------- |
| `id`, `sessionId`, `name` | The session                                        |
| `nameSource`, `cwd`       | As for a past session                              |
| `size`                    | The bytes it takes                                 |
| `lastActive`              | When its transcript last changed                   |
| `why`                     | Its `session gc` kind, `review`, or why it is kept |
| `lastPrompt`, `activity`  | As for a past session                              |

An older copy or a leftover in `remove` has only `id`, `sessionId`, `name`,
`cwd`, `size`, and `why`. Its `id` is empty, since `session gc ID` removes the
whole session: remove it with `session gc --copies` or `--leftovers`. The
`session gc` plan shows these, and the saved outputs `--sidecars` trims, with no
id too.

#### Teams

`team ls` prints a list of:

| Field             | Meaning                                |
| ----------------- | -------------------------------------- |
| `team`            | The team's name                        |
| `id`, `sessionId` | The lead's short job id and session id |
| `lead`, `cwd`     | The lead's name and directory          |
| `running`         | Whether the lead runs                  |
| `teammates`       | The members' names                     |
| `tasks`           | The task list                          |

Each task has `id`, `subject`, `status`, `owner`, and `blockedBy`.

#### MCP servers

`mcp ls` prints one object:

| Field     | Meaning                                                     |
| --------- | ----------------------------------------------------------- |
| `dir`     | The directory whose servers it lists                        |
| `profile` | The profile `--profile` named, or empty                     |
| `servers` | The servers: on first, then pending, then off, then by name |

Each server has:

| Field          | Meaning                                                          |
| -------------- | ---------------------------------------------------------------- |
| `name`         | Its name; a plugin's server is `plugin:PLUGIN:SERVER`            |
| `scope`        | `user`, `local`, `project`, `plugin`, or `managed`               |
| `state`        | `on`, `off`, or `pending` for a project server not yet approved  |
| `kind`         | The transport, such as `stdio`, `http`, or `sse`, or empty       |
| `profiles`     | The profiles in `mcp.profiles` that keep it on                   |
| `profileState` | Its state under `--profile`: `on` or `off`, or empty without one |

`mcp profiles` prints a list of profiles, each with `name`, `servers`, and
`unknown`: the servers that match none claudes can find.

#### Checks

`doctor` prints a list of checks, each with `name`, `result` (`ok`, `warn`, or
`fail`), and `detail`. `doctor env` prints its sections, `claude`, `claudes`,
`environment`, and `files`, as keys and values.

#### Reports

`restart`, `pause`, `resume`, `ping`, `stop`, `gc`, `pin`, `unpin`, and
`session gc` print one report when they finish:

| Field        | Meaning                                                           |
| ------------ | ----------------------------------------------------------------- |
| `command`    | The command, such as `restart` or `session gc`                    |
| `dryRun`     | Whether it was a dry run                                          |
| `plan`       | The sessions it set out to act on                                 |
| `results`    | What happened to each                                             |
| `leftOut`    | The sessions it chose not to act on, and why                      |
| `manual`     | Commands to run by hand, such as resuming an interactive session  |
| `failures`   | The sessions that need attention, and why                         |
| `notReached` | The sessions a Ctrl-C left as they were                           |
| `warnings`   | What it warned about, such as teammates that stop with their lead |
| `error`      | Why it stopped, or empty                                          |

Each session in those lists has `id`, `name`, `cwd`, `result`, `note` (such as
`will be pinged`), `command` (a resume command to run by hand), and `error`.
`result` says what happened, such as `restarted` or `failed`, or why the session
was left out: `pinned`, `interactive`, `leads a team`, `skipped`, or
`no job id`. `resume --forget` reports each dropped session as `forgotten`. In
`session gc`'s plan, `result` is the kind, such as `empty` or `copy`, and `note`
the size; each result is `trashed`, `deleted`, `skipped`, or `failed`. Warnings
and errors go to stderr as well.

### Colors

Text output is colored when it goes to a terminal: session status, pins,
outdated versions, and directories in the tables, what happened to each session,
failures and errors, and each `doctor` result. Stdout and stderr are checked
apart, so a pipe gets plain text. `json`, `yaml`, completions, `init`, and
`skill` are never colored.

Environment variables change that for text output, as the
[clicolors convention](https://bixense.com/clicolors/) says. claudes reads these
variables in this order, and the first that applies wins. An empty variable
counts as unset.

1. `NO_COLOR`, set to any value, turns colors off.
2. `CLICOLOR_FORCE` or `FORCE_COLOR`, set to any value but `0`, turns colors on,
   even for a pipe. Many CI systems and Node tools set `FORCE_COLOR`.
3. `CLICOLOR=0` turns colors off.
4. Otherwise a terminal that shows colors gets them, unlike one with
   `TERM=dumb`, and a pipe doesn't. `CLICOLOR=1`, which many macOS users export
   for `ls`, changes nothing here.

## Exit codes

| Code | Meaning                                                                        |
| ---- | ------------------------------------------------------------------------------ |
| 0    | Done                                                                           |
| 1    | Failed, such as a bad flag, a `--wait` that timed out, or a failed `doctor`    |
| 2    | A target named no session, or several where one was needed                     |
| 3    | Some sessions failed; the rest were handled, and the output says which         |
| 4    | Refused before any change: the wrong kind of session, or a failed safety check |
| 5    | Another claudes was changing sessions, so this one changed nothing             |
| 130  | Ctrl-C, or SIGTERM or SIGHUP on Unix; some sessions may already be stopped     |

[When something goes wrong](#when-something-goes-wrong) says what to do next.

## When something goes wrong

The exit code says what happened, and the output names each session:

1. **Exit 3, some sessions failed.** Each failure says why, and a session
   stopped for a restart or ping stays parked. Run `claudes resume --dry-run` to
   see them, then `claudes resume` to bring them back.
2. **Exit 4, refused.** Nothing changed. The message names the session and why,
   such as an interactive one for `ping`, or a stopped one for `stop`. A
   compatibility refusal means Claude Code's output changed: run
   `claudes doctor`, and pass `--force` only once you know why.
3. **Exit 5, another claudes is running.** Wait for it, then run the command
   again.
4. **Exit 130, Ctrl-C, SIGTERM, or SIGHUP.** The sessions already handled are
   listed. Run `claudes ls -a` and `claudes resume --dry-run` to see where the
   rest are.
5. **A session didn't come back.** When claudes says Claude Code accepted the
   resume but the session isn't running yet, check `claudes ls` before resuming
   it again: resuming a running session starts a copy.

Add `-v` to see each `claude` call claudes makes, and attach
`claudes doctor env` to a bug report.

## Configuration

`config.yaml` sets defaults that flags would otherwise set. Every key is
optional, and a flag always wins.

```yaml
ping:
  text: Carry on where you stopped. # what --ping and ping send
wait:
  timeout: 45m # how long --wait waits; 30m by default
output:
  format: json # the default -o of ls, where, team ls, doctor, mcp ls, mcp profiles, and the session commands
  dirs: fish # how text tables shorten a long directory: middle, fish, or full
mcp:
  profiles: # each profile, and the MCP servers that stay on in it
    safe: [browser, "plugin:docs:search"]
```

`claudes config get` lists every key with its value, and shows `mcp.profiles` as
one line of JSON. `claudes config set KEY VALUE` changes one and keeps the
file's comments, except `mcp.profiles`, which takes `claudes config edit`. A
profile's name is letters, digits, `-`, and `_`, and each server name is a
string; quote one that YAML would read as a number or `true`. `output.dirs`
takes `middle`, the default, `fish`, or `full`
([Directories in tables](#directories-in-tables)). `claudes config edit` opens
the file in `$VISUAL` or `$EDITOR`, and `claudes config path` prints where it
is. claudes refuses a file with an unknown key or a value it cannot use. Until
it is fixed, only `config`, `doctor`, `init`, `completion`, `skill`,
`--version`, and help run.

## Compatibility

claudes reads `claude agents --json`, files under `~/.claude/jobs`, and the
transcripts under `~/.claude/projects`, and none of them is a stable Claude Code
interface. So before a command changes anything, claudes checks what it read:

- A session kind, status, job id, or session id it doesn't recognize, or an
  entry in `pins.json` that isn't a job id, stops the command before it acts.
  `--force` goes ahead anyway, with a warning.
- A Claude Code outside the range claudes was tested with gets a warning, once
  per run. The installed version is cached for six hours.

`claudes mcp` reads Claude Code's MCP configuration, which is not a stable
interface either, and changes nothing. A field it cannot read shows `-`, and a
file it cannot read gets a warning, so it never fails on them.

`claudes doctor` runs every check and says what failed. It also warns when a
flag in Claude Code's own settings file, `~/.claude.json`, says Claude Code may
write `pins.json` without the lock claudes takes. Pinning from agent view and
from claudes at the same moment could then lose one of the pins. `--cli-only`
checks only the version and that `claude --help` and `claude agents --help`
still list every command and flag claudes calls, and `--strict` fails on
warnings too. `claudes doctor env` prints the versions, file locations, and
environment variables for a bug report.

## Logging

claudes logs nothing by default. `-v` (`--verbose`) prints a debug log to
stderr: each `claude` call with its arguments, directory, time, and result, and
the steps a command takes, such as the run lock and each wait.
`CLAUDES_LOG=debug` does the same without the flag, and `info`, `warn`, or
`error` log less. What a command reports to you still goes to stdout and stderr
as usual. `--version` prints the version.

## Files

| File                  | Linux and macOS                                        | Windows                        |
| --------------------- | ------------------------------------------------------ | ------------------------------ |
| `config.yaml`         | `$XDG_CONFIG_HOME/claudes`, or `~/.config/claudes`     | `%APPDATA%\claudes`            |
| `parked.json`         | `$XDG_STATE_HOME/claudes`, or `~/.local/state/claudes` | `%LOCALAPPDATA%\claudes\state` |
| `moves.json`          | `$XDG_STATE_HOME/claudes`, or `~/.local/state/claudes` | `%LOCALAPPDATA%\claudes\state` |
| `run.lock`            | `$XDG_STATE_HOME/claudes`, or `~/.local/state/claudes` | `%LOCALAPPDATA%\claudes\state` |
| `claude-version.json` | `$XDG_CACHE_HOME/claudes`, or `~/.cache/claudes`       | `%LOCALAPPDATA%\claudes\cache` |

- `parked.json` lists paused sessions, and is deleted when it's empty. On
  Windows, claudes also reads one in `~/.local/state/claudes`, and moves it
  under `%LOCALAPPDATA%` the first time it changes the list.
- `moves.json` maps each copy `session mv` made to the directory it belongs to.
  When `session mv` removed the session's stopped background job, the entry is
  an object, `{"dir": DIR, "options": [...]}`, with the options Claude Code had
  saved for the job. `session resume --bg` passes them to the new job.
- `run.lock` is a directory that exists only while a command changes sessions.
  Its `owner` file holds the PID and start time of the claudes that holds it. A
  lock that has not been refreshed for 30 seconds is broken only once that
  process has ended, or when it has no `owner` file.
- An `XDG_` variable that holds an absolute path moves its directory on every
  platform, Windows included. `CLAUDES_CONFIG` names another `config.yaml`, and
  must be an absolute path.
- `CLAUDE_CONFIG_DIR` moves `~/.claude`, as it does for Claude Code, and must be
  an absolute path. When background sessions run but its `jobs` directory is
  missing, commands that change sessions stop rather than read pins from the
  wrong place.
- claudes itself writes under `~/.claude` only to pin and unpin, as agent view
  does, for `session mv`, which adds a copy, for `skill --install`, and for
  `gc --teams`, which moves leftover team and task directories to the system
  trash. Unpinning also deletes the job's `order` file, as agent view does.
  Everything else it changes goes through the `claude` command.
