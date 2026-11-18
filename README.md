# claudes

Manage your Claude Code sessions from the command line: list, restart, pause,
resume, follow, and find them.

![claudes listing sessions, restarting and pinging the busy ones while the
pinned one stays put, pausing and resuming them, then sizing and cleaning up
past sessions](docs/demo/demo.gif)

You run Claude Code sessions in the background (`claude --bg`), and
`claude agents` lists them. Then Claude Code updates, and every running session
stays on the old version until it restarts. Or you reboot, and every session
dies with whatever it was doing. Agent view acts on one session at a time, and a
plain restart drops the turn a session was in the middle of.

claudes does those jobs in one command, without losing work:

```console
$ claudes restart --outdated --ping --dry-run
Restarting 2 session(s):
  ID        STATUS  DIR                 NAME          NOTE
  034d46b9  busy    ~/src/api-refactor  api-refactor  will be pinged
  51572bb7  idle    ~/src/docs          docs

Dry run: nothing was changed.

Pinned, not restarted: release-notes. Name a session, or add --pinned, to include it.

$ claudes restart --outdated --ping
Restarting 2 session(s):
  ID        STATUS  DIR                 NAME          NOTE
  034d46b9  busy    ~/src/api-refactor  api-refactor  will be pinged
  51572bb7  idle    ~/src/docs          docs
  restarted  api-refactor and pinged
  restarted  docs
```

`api-refactor` was mid-turn, so claudes stopped it, resumed it on the new Claude
Code, and sent it a prompt to pick up where it left off. The pinned session
stayed as it was. claudes also finds every past session, running or not, and
resumes or moves it from any directory. Anything it doesn't handle goes straight
to `claude agents`.

```console
$ claudes ls
ID        PINNED  KIND  STATE    STATUS  AGENTS     AGE  VERSION  DIR            NAME
3ec56511  yes     bg    working  busy    lead+2, 3  3h   2.1.325  ~/src/api      api-refactor
ff78d735  -       bg    done     idle    -          5h   2.1.325  ~/src/claudes  claudes-mvp
e89ec2c4  -       tty   -        idle    -          20m  -        ~/src/claudes  e89ec2c4
```

`AGENTS` shows the agent team a session leads, as `lead+2`, and the subagents it
runs now, as `3`. `claudes ls --wide` adds its background shells, tokens, pull
requests, and what it's doing.

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

macOS and Linux:

```bash
curl -fsSL https://github.com/uvw/claudes/releases/latest/download/install.sh | sh
```

Windows, in PowerShell:

```powershell
irm https://github.com/uvw/claudes/releases/latest/download/install.ps1 | iex
```

The script checks the download against the release's checksums and, with the
[GitHub CLI](https://cli.github.com) signed in, its build provenance, then
installs into `~/.local/bin`, or `%LOCALAPPDATA%\Programs\claudes` on Windows.
Run `claudes upgrade` later to move to a newer release.

With Homebrew on macOS or Linux, winget on Windows (once Microsoft has approved
its first version), or Go:

```bash
brew install uvw/tap/claudes
winget install uvw.claudes
go install github.com/uvw/claudes/cmd/claudes@latest
```

[Installing claudes](https://github.com/uvw/claudes/blob/main/docs/install.md)
covers picking a version or a directory, installing by hand, checking a
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

A skill teaches a coding agent to use claudes, so you can ask it to "pin the
docs session" or "restart my agents after the update". It carries the install
steps too, so an agent can set claudes up itself.

```bash
claudes skill --install           # install or update it for Claude Code
npx skills add uvw/claudes        # or install it from the repository
claudes skill                     # print it
```

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
| `cd TARGET`                      | Change to a session's directory (needs `init`)           |

Setup and diagnostics:

| Command                      | What it does                                                |
| ---------------------------- | ----------------------------------------------------------- |
| `config`                     | Show or change settings in `config.yaml`                    |
| `mcp ls [TARGET]`            | List a session's MCP servers, and whether each is on        |
| `mcp profiles`               | List the MCP profiles in `config.yaml`                      |
| `mcp on\|off PATTERN TARGET` | Turn MCP servers on or off in a running session             |
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
| [Agent skill](skills/claudes/SKILL.md)                                                       | How a coding agent uses claudes, as a skill                                   |
| [Security](https://github.com/uvw/claudes/blob/main/docs/security.md)                        | Reporting a problem, and checking a download                                  |
| [Contributing](https://github.com/uvw/claudes/blob/main/docs/contributing.md)                | Rules for changes, the checks, and releases                                   |
| [Agents](AGENTS.md)                                                                          | What coding agents need on top of the contributing guide                      |
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

[Decision 29](docs/decisions.md#29-claudes-is-a-tool-for-claude-code-and-nothing-else)
says why.

## License

[MIT](LICENSE)
