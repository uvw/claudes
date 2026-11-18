# Claudes

**A pocketknife for your Claude Code sessions.**

You run Claude Code in three terminals and a dozen background sessions. Claude
Code updates, and every one of them stays on yesterday's version. You reboot,
and they all die mid-thought. claudes restarts or parks them in one command,
skips the pinned ones, and nudges any caught mid-turn to carry on.

![claudes marking outdated sessions (old), restarting them and pinging the busy
one while the pinned one stays put, pausing and resuming sessions, searching
past sessions for a lock race, listing running sessions and those stopped today,
sorting past sessions with triage, and previewing a session gc
cleanup](docs/demo/demo.gif)

## Use cases

### Claude Code just updated

Your sessions still run the old version, and `claudes ls` marks them `(old)`.
Restart those, and ping the ones that were busy:

```console
$ claudes ls
ID        PINNED  KIND  STATE    STATUS  AGENTS  AGE  VERSION        DIR                  NAME
c41d7e93  yes     bg    done     idle    -       40m  2.1.322 (old)  ~/src/release-notes  release-notes
5e9b2a64  -       bg    working  busy    -       15m  2.1.325        ~/src/flaky-tests    flaky-tests
3f2a9c41  -       bg    working  busy    3       1h   2.1.322 (old)  ~/src/api-refactor   api-refactor
8b7e1d02  -       bg    done     idle    -       4h   2.1.322 (old)  ~/src/docs           docs

(old): behind the installed Claude Code, see claudes restart --outdated

$ claudes restart --outdated --ping
Restarting 2 session(s):
  ID        STATUS  DIR                 NAME          NOTE
  3f2a9c41  busy    ~/src/api-refactor  api-refactor  will be pinged
  8b7e1d02  idle    ~/src/docs          docs
  restarted  api-refactor and pinged
  restarted  docs

Pinned, not restarted: release-notes. Add --pinned to include them. Named sessions replace the bulk selection, so name every session you want.
```

`api-refactor` was mid-turn with three subagents at work. claudes stopped it,
resumed it on the new version, and told it to pick up where it left off. The
pinned `release-notes` stayed put, and `flaky-tests`, already current, kept
working. Add `--dry-run` to see the plan first.

### Rebooting

A reboot kills every session. Park them first, and bring them back after:

```bash
claudes pause            # stop background sessions and remember them
# reboot
claudes resume --ping    # bring them back; the mid-turn ones carry on
```

`pause` skips pinned sessions, interactive ones, and leads of an agent team, and
a reboot ends those too. `--pinned` and `--interactive` take the first two. A
lead goes only when you name it, along with every other session you want. After
the reboot, `claudes resume NAME` brings back any stopped background session,
parked or not, and the resume commands `pause` printed bring back interactive
ones.

### Finding an old session

"Which session fixed that lock race on Monday?" Search every session, running or
long finished, by what was said in it:

```console
$ claudes session search lock race -l week
ID        ACTIVE  WHERE   DIR           NAME         MATCH
a1b2c3d4  1d      you     ~/src/api     cache-fix    …I think it is a lock race in the writer
b2c3d4e5  2d      claude  ~/src/ingest  ingest-perf  The lock is held across the network call. A race shows up only under…

Resume the first with:
  claudes session resume a1b2c3d4
```

`session resume` works from any directory: it starts the session in its own.

### Moving a session

You started in `~/scratch`, and the work grew into a repository at
`~/src/billing`. Move the session so it resumes there:

```bash
claudes session mv billing-spike ~/src/billing
```

It copies the transcript, keeps the original, and prints the commands that
resume the copy and remove the original.

### Clearing out old transcripts

Transcripts pile up under `~/.claude` by the gigabyte. See where the space went,
sort out what's worth keeping, and sweep the rest:

```bash
claudes session du --top 10     # disk use by directory, and the 10 biggest
claudes session triage          # sort sessions into remove, review, and keep
claudes session gc --dry-run    # empty sessions, old copies, and leftovers
claudes session gc              # move them to the system trash
```

They go to the trash, not the void, and `claudes session restore ID` puts one
back where it came from. `triage` prints the `session gc` command for its remove
pile, and the [agent skill](#agent-skill) teaches an agent to judge the review
pile for you.

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
claudes session resume api       # resume a past session in its own directory
claudes cd api                   # jump to a session's directory (see Shell setup)
claudes doctor                   # check claudes against the installed Claude Code
```

`restart`, `pause`, `resume`, and `gc` take `--dry-run` to show their plan
first. `ping`, `stop`, `pin`, and `unpin` act on the sessions you name.

## Install

claudes needs Claude Code with `claude agents` on `PATH`. `claudes doctor` names
the Claude Code versions claudes was tested with, and says whether yours works.

macOS and Linux:

```bash
curl -fsSL https://github.com/uvw/claudes/releases/latest/download/install.sh | sh
```

Windows, in PowerShell:

```powershell
irm https://github.com/uvw/claudes/releases/latest/download/install.ps1 | iex
```

The script checks the download and installs it into `~/.local/bin`, or
`%LOCALAPPDATA%\Programs\claudes` on Windows. `claudes upgrade` moves to a newer
release later. With Homebrew on macOS or Linux, or with Go:

```bash
brew install uvw/tap/claudes
go install github.com/uvw/claudes/cmd/claudes@latest
```

[Installing claudes](https://github.com/uvw/claudes/blob/main/docs/install.md)
covers winget, picking a version or a directory, installing by hand, checking a
download, and uninstalling. Then run `claudes doctor`.

### Shell setup

`claudes init` adds the shell function behind `claudes cd NAME`, and
`claudes completion` adds tab completion that offers session names. Add the
`init` line for your shell to its profile:

```bash
eval "$(claudes init zsh)"     # ~/.zshrc
eval "$(claudes init bash)"    # ~/.bashrc
claudes init fish | source     # ~/.config/fish/config.fish
claudes init pwsh | Out-String | Invoke-Expression   # $PROFILE
```

[Shell setup](https://github.com/uvw/claudes/blob/main/docs/install.md#shell-setup)
has the completion lines for each shell, and zsh's `noglob` alias, which keeps a
pattern such as `plan*` away from the shell.

### Agent skill

`claudes skill --install` installs a skill that teaches a coding agent to use
claudes, so you can ask it to "pin the docs session" or "restart my agents after
the update".
[Agent skill](https://github.com/uvw/claudes/blob/main/docs/install.md#agent-skill)
covers the other ways to install it.

## Commands

`claudes` with no command runs `claude agents`, and so does anything that is not
a claudes command. `claudes COMMAND --help` describes each command's flags, and
[Using claudes](https://github.com/uvw/claudes/blob/main/docs/guide.md) explains
how they act and keep work safe.

Running sessions, as agent view lists them, plus finished background ones with
`-a`:

| Command               | What it does                                                   |
| --------------------- | -------------------------------------------------------------- |
| `ls [TARGET...]`      | List them: pinned first, then busy, then newest                |
| `where TARGET...`     | Print the directories of the sessions that match               |
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

Every session with a transcript, past or running (`claudes s` and
`claudes sessions` are short for `claudes session`):

| Command                          | What it does                                             |
| -------------------------------- | -------------------------------------------------------- |
| `session ls [TARGET...]`         | List them, newest activity first, with their size        |
| `session search QUERY...`        | Find them by their prompts, replies, and tool calls      |
| `session show TARGET`            | Directory, branch, version, times, size, and last prompt |
| `session where TARGET...`        | Print their directories                                  |
| `session resume TARGET [PROMPT]` | Resume one in its own directory, from anywhere           |
| `session mv TARGET DIR`          | Copy one to another directory, so it resumes there       |
| `session du`                     | Disk space by directory; `--top N` for the biggest       |
| `session triage`                 | Sort them into remove, review, and keep                  |
| `session gc [TARGET...]`         | Move empty, copied, and leftover sessions to the trash   |
| `session restore [TARGET...]`    | Put back what claudes moved to the trash                 |
| `cd TARGET`                      | Change to a session's directory (needs `init`)           |

Setup and diagnostics:

| Command                      | What it does                                                |
| ---------------------------- | ----------------------------------------------------------- |
| `config`                     | Show or change settings in `config.yaml`                    |
| `mcp ls [TARGET]`            | List a session's MCP servers, and whether each is on        |
| `mcp profiles`               | List the MCP profiles in `config.yaml`                      |
| `mod install`, `uninstall`   | Add or remove the claudes mod, which reaches into sessions  |
| `mod status`                 | Whether the mod is installed, and which sessions loaded it  |
| `doctor`                     | Check the installed Claude Code against claudes             |
| `doctor env`                 | Print versions, file locations, and the variables           |
| `init bash\|zsh\|fish\|pwsh` | Print the shell function behind `claudes cd`                |
| `skill`                      | Print or install the agent skill                            |
| `completion SHELL`           | Print the completion script for bash, zsh, fish, powershell |
| `upgrade`                    | Install the latest release in place of this claudes         |

## Documentation

| Document                                                                                     | What it covers                                                                |
| -------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------- |
| [Installing claudes](https://github.com/uvw/claudes/blob/main/docs/install.md)               | Every way to install, shell setup, the agent skill, the mod, and uninstalling |
| [Using claudes](https://github.com/uvw/claudes/blob/main/docs/guide.md)                      | Naming sessions, flags, safety, past sessions, teams, logs, and MCP           |
| [Reference](https://github.com/uvw/claudes/blob/main/docs/reference.md)                      | Output, exit codes, troubleshooting, configuration, and files                 |
| [Agent skill](https://github.com/uvw/claudes/blob/main/skills/claudes/SKILL.md)              | How a coding agent uses claudes, as a skill                                   |
| [Security](https://github.com/uvw/claudes/blob/main/docs/security.md)                        | Reporting a problem, and checking a download                                  |
| [Contributing](https://github.com/uvw/claudes/blob/main/docs/contributing.md)                | Rules for changes, the checks, and releases                                   |
| [Agents](https://github.com/uvw/claudes/blob/main/AGENTS.md)                                 | What coding agents need on top of the contributing guide                      |
| [Decisions](https://github.com/uvw/claudes/blob/main/docs/decisions.md)                      | Each settled choice, why, and the alternatives                                |
| [Releasing](https://github.com/uvw/claudes/blob/main/docs/releasing.md)                      | How releases happen, recovering one, and repo settings                        |
| [Changelog](https://github.com/uvw/claudes/blob/main/docs/changelog.md)                      | What each release changed                                                     |
| [Handoffs](https://github.com/uvw/claudes/blob/main/docs/handoffs.md)                        | Work one machine leaves for another, and a resume prompt                      |
| [Third-party notices](https://github.com/uvw/claudes/blob/main/docs/third-party-notices.txt) | The licenses of the Go modules built into the binary                          |

## Development

You need Go at the version `go.mod` names; with `GOTOOLCHAIN=auto`, the default,
`go` fetches it.
[docs/contributing.md](https://github.com/uvw/claudes/blob/main/docs/contributing.md#checks)
lists the checks to run before a commit.

```bash
go test ./...
goreleaser build --snapshot --clean
```

### Goals and non-goals

claudes is a tool: a pocketknife for the Claude Code command line. Its goals:

1. Handle the jobs around running Claude Code sessions: restarting, pausing,
   resuming, finding, moving, and cleaning up after them.
2. Act through the `claude` command and the files Claude Code keeps, and pass
   anything else to `claude agents`.
3. Stay safe to run against live work, as one small binary with no service
   behind it.

Its non-goals:

1. Growing into an agent orchestrator or a multi-agent platform. claudes does
   not plan work, split it across agents, or decide what a session does next.
2. Other coding agents, such as Copilot, Codex, or Pi. claudes manages Claude
   Code only.

[Decision 29](https://github.com/uvw/claudes/blob/main/docs/decisions.md#29-claudes-is-a-tool-for-claude-code-and-nothing-else)
says why.

## License

[MIT](https://github.com/uvw/claudes/blob/main/LICENSE)
