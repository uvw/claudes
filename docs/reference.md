# Reference

Output formats, exit codes, what to do when something goes wrong, configuration,
compatibility checks, logging, and the files claudes keeps.

## Output

`ls`, `where`, `team ls`, `session ls`, `session show`, `session where`,
`session du`, `session triage`, `doctor`, and `doctor env` take `-o text`,
`-o json`, or `-o yaml`; `ls --json` is short for `-o json`. When `where`
matches one session, `text` prints only its folder, so
`cd "$(claudes where NAME)"` works; add `-o text` when `output.format` is set.
`claudes init` does that itself. `logs` takes `-o text` or `-o json`, which
prints the transcript records themselves, one per line.

`json` and `yaml` use the camelCase field names of `claude agents --json`, and
every key is always present:

- A session from `ls` or `where` has `id`, `sessionId`, `name`, `cwd`, `kind`,
  `state`, `status`, `pid`, and `startedAt`, plus `pinned`, `version`,
  `outdated`, and `teammates`. An interactive session has no job id, so its `id`
  is empty; the table shows its short session id instead.
- A session from `session ls` or `session show` has `id`, `sessionId`, `name`,
  `nameSource` (`custom` for `/rename`, `agent`, `ai`, or empty), `cwd`,
  `branch`, `started`, `lastActive`, `lastPrompt`, `version`, `path`, `copies`
  (its other transcripts, after a `session mv`), `size`, `running`, and
  `activity`. `activity` is null in `session ls`. In `session show` it has
  `prompts`, `commands`, `toolCalls`, `edits`, `firstPrompt`, and `teammate`.
- `session du` prints `folders`, each with `folder`, `project`, `sessions`, and
  `disk`; `top`, the biggest sessions with `--top`, each with `id`, `sessionId`,
  `name`, `cwd`, `running`, and `disk`; `leftovers`; and `total`.
- `session triage` prints `remove`, `review`, and `keep`, each a list of
  sessions with `id`, `sessionId`, `name`, `nameSource`, `cwd`, `size`,
  `lastActive`, `why` (the kind `session gc` removes it as, `review`, or why it
  is kept), `lastPrompt`, and `activity`. Folders in `remove` have only `id`,
  `sessionId`, `name`, `cwd`, `size`, and `why`.
- A team from `team ls` has `team`, `id`, `sessionId`, `lead`, `cwd`, `running`,
  `teammates`, and `tasks`, each task with `id`, `subject`, `status`, `owner`,
  and `blockedBy`.
- A `doctor` check has `name`, `result` (`ok`, `warn`, or `fail`), and `detail`.
  `doctor env` prints sections of keys and values.

`restart`, `pause`, `resume`, `ping`, `stop`, `gc`, `pin`, and `unpin` take `-o`
too, and print one report when they finish: `command`, `dryRun`, then lists of
sessions under `plan`, `results`, `leftOut`, `manual`, `failures`, and
`notReached`. Each entry has `id`, `name`, `cwd`, `result` (such as `restarted`,
`failed`, or why one was left out: `pinned`, `interactive`, `leads a team`,
`skipped`, or `no job id`), `note`, `command` (a resume command to run by hand),
and `error`. `resume --forget` lists each dropped session in `results` as
`forgotten`. `warnings` lists what the command warned about, such as teammates
that stop with their lead, and `error` is why it stopped, or empty. Warnings and
errors still go to stderr as well. These commands default to `text`, whatever
`output.format` says.

### Colors

Text output is colored when it goes to a terminal: session status, pins,
outdated versions, and folders in the tables, what happened to each session,
failures and errors, and each `doctor` result. Stdout and stderr are checked
apart, so a pipe gets plain text. `NO_COLOR`, set to any value but empty, turns
colors off. `CLICOLOR=0` turns them off too, and `CLICOLOR=1` turns them on even
for a pipe. `json`, `yaml`, completions, `init`, and `skill` are never colored.

## Exit codes

| Code | Meaning                                                                        |
| ---- | ------------------------------------------------------------------------------ |
| 0    | Done                                                                           |
| 1    | Failed, such as a bad flag, a `--wait` that timed out, or a failed `doctor`    |
| 2    | A target named no session, or several where one was needed                     |
| 3    | Some sessions failed; the rest were handled, and the output says which         |
| 4    | Refused before any change: the wrong kind of session, or a failed safety check |
| 5    | Another claudes was changing sessions, so this one changed nothing             |
| 130  | Ctrl-C; some sessions may already be stopped                                   |

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
4. **Exit 130, Ctrl-C.** The sessions already handled are listed. Run
   `claudes ls -a` and `claudes resume --dry-run` to see where the rest are.
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
  format: json # the default -o of ls, where, team ls, and the session commands
```

`claudes config get` lists every key with its value,
`claudes config set KEY VALUE` changes one and keeps the file's comments,
`claudes config edit` opens the file in `$VISUAL` or `$EDITOR`, and
`claudes config path` prints where it is. claudes refuses a file with an unknown
key or a value it cannot use. Until it is fixed, only `config`, `doctor`,
`init`, `completion`, `skill`, `--version`, and help run.

## Compatibility

claudes reads `claude agents --json`, files under `~/.claude/jobs`, and the
transcripts under `~/.claude/projects`, and none of them is a stable Claude Code
interface. So before a command changes anything, claudes checks what it read:

- A session kind, status, job id, or session id it doesn't recognize, or an
  entry in `pins.json` that isn't a job id, stops the command before it acts.
  `--force` goes ahead anyway, with a warning.
- A Claude Code outside the range claudes was tested with gets a warning, once
  per run. The installed version is cached for six hours.

`claudes doctor` runs every check and says what failed. `--cli-only` checks only
the version and that `claude --help` and `claude agents --help` still list every
command and flag claudes calls, and `--strict` fails on warnings too.
`claudes doctor env` prints the versions, file locations, and environment
variables for a bug report.

## Logging

claudes logs nothing by default. `-v` (`--verbose`) prints a debug log to
stderr: each `claude` call with its arguments, folder, time, and result, and the
steps a command takes, such as the run lock and each wait. `CLAUDES_LOG=debug`
does the same without the flag, and `info`, `warn`, or `error` log less. What a
command reports to you still goes to stdout and stderr as usual. `--version`
prints the version.

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
- `moves.json` maps each copy `session mv` made to the folder it belongs to.
- `run.lock` is a folder that exists only while a command changes sessions.
- An `XDG_` variable that holds an absolute path moves its folder on every
  platform, Windows included. `CLAUDES_CONFIG` names another `config.yaml`, and
  must be an absolute path.
- `CLAUDE_CONFIG_DIR` moves `~/.claude`, as it does for Claude Code, and must be
  an absolute path. When background sessions run but its `jobs` folder is
  missing, commands that change sessions stop rather than read pins from the
  wrong place.
- claudes itself writes under `~/.claude` only to pin and unpin, as agent view
  does, for `session mv`, which adds a copy, for `skill --install`, and for
  `gc --teams`, which removes leftover team and task folders. Unpinning also
  deletes the job's `order` file, as agent view does. Everything else it changes
  goes through the `claude` command.
