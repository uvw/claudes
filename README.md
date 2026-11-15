# claudes

Manage your Claude Code sessions from the command line: list, restart, pause,
resume, follow, and find them.

You run Claude Code sessions in the background (`claude --bg`), and
`claude agents` lists them. Then Claude Code updates, and every running session
stays on the old version until it restarts. Or you reboot, and every session
dies with whatever it was doing. Agent view acts on one session at a time, and a
plain restart drops the turn a session was in the middle of.

claudes does those jobs in one command, without losing work:

```console
$ claudes restart --outdated --ping --dry-run
Restarting 2 session(s):
  034d46b9  api-refactor                busy      ~/src/api-refactor  (will be pinged)
  51572bb7  docs                        idle      ~/src/docs

Dry run: nothing was changed.

Pinned, not restarted: release-notes. Name a session to include it.

$ claudes restart --outdated --ping
Restarting 2 session(s):
  034d46b9  api-refactor                busy      ~/src/api-refactor  (will be pinged)
  51572bb7  docs                        idle      ~/src/docs
  restarted  api-refactor and pinged
  restarted  docs
```

`api-refactor` was mid-turn, so claudes stopped it, resumed it on the new Claude
Code, and sent it a prompt to pick up where it left off. The pinned session
stayed as it was. claudes also finds every past session, running or not, and
resumes or moves it from any folder. Anything it doesn't handle goes straight to
`claude agents`.

```console
$ claudes ls
PINNED  ID        NAME          KIND  STATE    STATUS  TEAM    AGE  VERSION  CWD
yes     3ec56511  api-refactor  bg    working  busy    lead+2  3h   2.1.325  ~/src/api
-       ff78d735  claudes-mvp   bg    done     idle    -       5h   2.1.325  ~/src/claudes
-       e89ec2c4  e89ec2c4      tty   -        idle    -       20m  -        ~/src/claudes
```

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

## Quick start

```bash
claudes                          # open agent view, as claude agents does
claudes ls plan                  # sessions with "plan" in the name
claudes ls -a                    # finished background sessions too
claudes restart --outdated --ping --dry-run   # see what an update restart does
claudes restart --outdated --ping             # then do it
claudes ping api "run the tests" # send one session a message
claudes logs -f api              # follow a session's prompts, replies, and tools
claudes stop api                 # stop one session
claudes session ls --since 2d    # every session from the last two days, past too
claudes session resume api       # resume a past session in its own folder
claudes cd api                   # jump to a session's folder (see Shell setup)
claudes doctor                   # check claudes against the installed Claude Code
```

`restart`, `pause`, `resume`, and `gc` take `--dry-run` to show their plan
first. `ping`, `stop`, `pin`, and `unpin` act on the sessions you name.

### Before and after a reboot

```bash
claudes pause --dry-run          # see what will be parked, and what won't
claudes pause                    # stop and park background sessions
# reboot
claudes resume --dry-run
claudes resume --ping            # bring them back; mid-turn ones carry on
```

`pause` leaves out pinned sessions, interactive ones, and leads of an agent
team, and a reboot ends them too. To keep them, name them
(`claudes pause api notes`) or add `--interactive`. After the reboot,
`claudes resume NAME` brings back any stopped background session, parked or not,
and the resume commands `pause` printed bring back interactive ones.

## Install

claudes needs Claude Code with `claude agents` on `PATH`. It is tested with
Claude Code 2.1.325, and `claudes doctor` says whether yours works with it.

Download the archive for your platform from the
[latest release](https://github.com/uvw/claudes/releases/latest), check it
against `checksums.txt`, and put `claudes` (`claudes.exe` on Windows) in a
folder on `PATH`. Archives cover macOS (`darwin`), Linux, and Windows, on
`amd64` and `arm64`. With the GitHub CLI on Linux:

```bash
gh release download --repo uvw/claudes \
  --pattern 'claudes_*_linux_amd64.tar.gz' --pattern checksums.txt
sha256sum --check --ignore-missing checksums.txt
tar -xzf claudes_*_linux_amd64.tar.gz claudes
install claudes ~/.local/bin/
```

On macOS, take the `darwin_arm64` archive, or `darwin_amd64` on Intel, and check
it with `shasum -a 256 --check --ignore-missing checksums.txt`. The binaries are
not signed, so one downloaded with a browser is blocked by Gatekeeper until you
run `xattr -d com.apple.quarantine claudes`. One fetched with `gh` or `curl` is
not.

On Windows, in PowerShell:

```powershell
gh release download --repo uvw/claudes `
  --pattern 'claudes_*_windows_amd64.zip' --pattern checksums.txt
$zip = Get-Item claudes_*_windows_amd64.zip
(Get-FileHash $zip -Algorithm SHA256).Hash.ToLower()  # compare with checksums.txt
$dir = "$env:LOCALAPPDATA\Programs\claudes"
Expand-Archive $zip -DestinationPath $dir
[Environment]::SetEnvironmentVariable('Path',
  [Environment]::GetEnvironmentVariable('Path', 'User') + ";$dir", 'User')
```

Open a new terminal for the `PATH` change to take.
[docs/security.md](docs/security.md) explains how to check a download's
provenance too.

With Go 1.21 or later, `go install` builds the latest release into
`$(go env GOPATH)/bin`, `~/go/bin` by default, and fetches the Go version
`go.mod` names if yours is older:

```bash
go install github.com/uvw/claudes/cmd/claudes@latest
```

Then run `claudes doctor`. To uninstall, delete the binary, and the folders in
[Files](#files) if you want its settings and park list gone too.

### Shell setup

Tab completion offers commands, flags, and the session names each command can
act on: `unpin` offers pinned sessions, `resume` parked and stopped ones, and
`gc` finished ones. `claudes init` adds a shell function so `claudes cd NAME`
changes your shell's folder, which no program can do on its own.

zsh: write the completion script once, to a folder on `fpath` you own, then add
the rest to `~/.zshrc`, before `compinit` runs:

```zsh
mkdir -p ~/.zfunc && claudes completion zsh > ~/.zfunc/_claudes  # once
```

```zsh
fpath=(~/.zfunc $fpath)
autoload -Uz compinit && compinit
eval "$(claudes init zsh)"
alias claudes='noglob claudes'
```

Keep the alias after the `init` line. It stops zsh from expanding a pattern such
as `plan*` against the files in the current folder before claudes sees it.

bash, in `~/.bashrc`. Completion needs the bash-completion package; on macOS's
bash 3.2, use `eval "$(claudes completion bash)"` instead of `source`:

```bash
source <(claudes completion bash)
eval "$(claudes init bash)"
```

fish, in `~/.config/fish/config.fish`, for completion only; `claudes cd` has no
fish function yet:

```fish
claudes completion fish | source
```

PowerShell, in `$PROFILE`:

```powershell
if (Get-Command claudes -ErrorAction SilentlyContinue) {
    Invoke-Expression (& { (claudes completion powershell | Out-String) })
    Invoke-Expression (& { (claudes init pwsh | Out-String) })
}
```

### Agent skill

A skill teaches a coding agent to use claudes, so you can ask it to "pin the
docs session" or "restart my agents after the update". It carries the install
steps too, so an agent can set claudes up itself.

```bash
claudes skill --install           # install or update it for Claude Code
npx skills add uvw/claudes        # or install it from the repository
claudes skill                     # print it
```

`--install` writes `~/.claude/skills/claudes/SKILL.md`, under
`CLAUDE_CONFIG_DIR` when that is set. `--dir PATH` writes
`PATH/claudes/SKILL.md`, for another agent's skills folder.

## Commands

`claudes` with no command, or with arguments that are not a claudes command,
runs `claude agents` with those arguments. So a mistyped command, such as
`claudes lst`, opens agent view rather than failing. `claudes -- ARGS` always
passes through, even when ARGS starts with a claudes command name, so a script
stays correct if claudes adds a command of the same name. In PowerShell with
`claudes init` loaded, quote it as `claudes '--' ARGS`: a PowerShell function
never receives a bare `--`. `claudes COMMAND --help` describes each command and
its flags.

Running sessions, as agent view lists them, plus finished background ones with
`-a`:

| Command               | What it does                                                   |
| --------------------- | -------------------------------------------------------------- |
| `ls [TARGET...]`      | List them: pinned first, then busy, then newest                |
| `where TARGET...`     | Print the folders of the sessions that match                   |
| `pin`, `unpin`        | Pin or unpin background sessions, as Ctrl+T does in agent view |
| `restart [TARGET...]` | Restart named sessions, or all but pinned ones                 |
| `pause [TARGET...]`   | Stop sessions and park them for `resume`                       |
| `resume [TARGET...]`  | Bring back parked sessions, or a stopped one                   |
| `ping TARGET [MSG]`   | Stop a background session and resume it with MSG               |
| `stop TARGET...`      | Stop sessions; `claudes resume` brings a background one back   |
| `logs TARGET[/MATE]`  | Recent output; `-f` follows the transcript                     |
| `attach TARGET`       | `claude attach`, by name                                       |
| `gc [TARGET...]`      | Take sessions finished over 7 days ago off the list            |
| `team ls [LEAD...]`   | Agent teams at work, with teammates and tasks                  |

A ping cuts off a turn in progress, and the session takes MSG as its next
prompt. With no MSG, `ping` sends "Pick up where you left off.".
`restart --ping` and `resume --ping` send "Claude Code was restarted while you
were working. Pick up where you left off." to the sessions that were mid-turn;
`ping.text` in [Configuration](#configuration) or `--ping-text` changes both.
`gc` removes through `claude rm`, which keeps each conversation, so
`session resume` still finds it, and deletes the session's worktree only when
that is safe.

Every session with a transcript, past or running:

| Command                          | What it does                                        |
| -------------------------------- | --------------------------------------------------- |
| `session ls [TARGET...]`         | List them, newest activity first                    |
| `session show TARGET`            | Folder, branch, version, times, and the last prompt |
| `session where TARGET...`        | Print their folders                                 |
| `session resume TARGET [PROMPT]` | Resume one in its own folder, from anywhere         |
| `session mv TARGET FOLDER`       | Copy one to another folder, so it resumes there     |
| `cd TARGET`                      | Change to a session's folder (needs `init`)         |

Setup and diagnostics:

| Command                | What it does                                                |
| ---------------------- | ----------------------------------------------------------- |
| `config`               | Show or change settings in `config.yaml`                    |
| `doctor`               | Check the installed Claude Code against claudes             |
| `doctor env`           | Print versions, file locations, and the variables           |
| `init bash\|zsh\|pwsh` | Print the shell function behind `claudes cd`                |
| `skill`                | Print or install the agent skill                            |
| `completion SHELL`     | Print the completion script for bash, zsh, fish, powershell |

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
   sessions; name one to include it.
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
9. **Unfamiliar data stops it.** See [Compatibility](#compatibility).
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

## Past sessions

`claudes session` reads the transcripts Claude Code keeps under
`~/.claude/projects`, so it finds every session, not only those agent view
lists. `session ls` takes `--in PATH` for sessions run in a folder and below it,
and `--since 7d` for recent ones. `session where --one` fails unless exactly one
session matches, for scripts. `session resume` refuses a session that is still
running, since resuming it would start a copy; `--bg` resumes in the background.

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

## Output

`ls`, `where`, `team ls`, `session ls`, `session show`, `session where`,
`doctor`, and `doctor env` take `-o text`, `-o json`, or `-o yaml`; `ls --json`
is short for `-o json`. When `where` matches one session, `text` prints only its
folder, so `cd "$(claudes where NAME)"` works; add `-o text` when
`output.format` is set. `claudes init` does that itself. `logs` takes `-o text`
or `-o json`, which prints the transcript records themselves, one per line.

`json` and `yaml` use the camelCase field names of `claude agents --json`, and
every key is always present:

- A session from `ls` or `where` has `id`, `sessionId`, `name`, `cwd`, `kind`,
  `state`, `status`, `pid`, and `startedAt`, plus `pinned`, `version`,
  `outdated`, and `teammates`. An interactive session has no job id, so its `id`
  is empty; the table shows its short session id instead.
- A session from `session ls` or `session show` has `id`, `sessionId`, `name`,
  `cwd`, `branch`, `started`, `lastActive`, `lastPrompt`, `version`, `path`,
  `copies` (its other transcripts, after a `session mv`), `size`, and `running`.
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

## Documentation

| Document                                            | What it covers                                           |
| --------------------------------------------------- | -------------------------------------------------------- |
| [Agent skill](skills/claudes/SKILL.md)              | How a coding agent uses claudes, as a skill              |
| [Security](docs/security.md)                        | Reporting a problem, and checking a download             |
| [Contributing](docs/contributing.md)                | Rules for changes, the checks, and releases              |
| [Agents](AGENTS.md)                                 | What coding agents need on top of the contributing guide |
| [Decisions](docs/decisions.md)                      | Each settled choice, why, and the alternatives           |
| [Releasing](docs/releasing.md)                      | How releases happen, recovering one, and repo settings   |
| [To do](docs/todo.md)                               | What blocks the first release; the rest is in issues     |
| [Handoffs](docs/handoffs.md)                        | Work one machine leaves for another, and a resume prompt |
| [Third-party notices](docs/third-party-notices.txt) | The licenses of the Go modules built into the binary     |

## Development

You need Go at the version `go.mod` names; with `GOTOOLCHAIN=auto`, the default,
`go` fetches it. [docs/contributing.md](docs/contributing.md#checks) lists the
checks to run before a commit.

```bash
go test ./...
goreleaser build --snapshot --clean
```

## License

[MIT](LICENSE)
