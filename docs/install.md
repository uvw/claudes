# Installing claudes

Every way to install claudes, set up your shell, and add the agent skill. The
[README](../README.md#install) has the short version.

claudes needs Claude Code with `claude agents` on `PATH`. `claudes doctor` names
the Claude Code versions claudes was tested with, and says whether yours works.

## The install script

macOS and Linux:

```bash
curl -fsSL https://github.com/uvw/claudes/releases/latest/download/install.sh | sh
```

Windows, in PowerShell:

```powershell
irm https://github.com/uvw/claudes/releases/latest/download/install.ps1 | iex
```

The script downloads the archive for your computer from the
[latest release](https://github.com/uvw/claudes/releases/latest) and checks it
against the release's `checksums.txt`. With the
[GitHub CLI](https://cli.github.com) installed and signed in, it also verifies
the archive's build provenance. It installs `claudes` into `~/.local/bin`, or
`claudes.exe` into `%LOCALAPPDATA%\Programs\claudes`, which it adds to your
`PATH`. Then it prints the optional next steps: [shell setup](#shell-setup), the
[agent skill](#agent-skill), and `claudes doctor`. Run it again to upgrade.

To pick a version or a directory:

```bash
curl -fsSL https://github.com/uvw/claudes/releases/latest/download/install.sh |
  sh -s -- --version v0.1.0 --dir /usr/local/bin
```

```powershell
$env:CLAUDES_VERSION = 'v0.1.0'; $env:CLAUDES_INSTALL_DIR = 'C:\Tools'
irm https://github.com/uvw/claudes/releases/latest/download/install.ps1 | iex
```

`CLAUDES_VERSION` and `CLAUDES_INSTALL_DIR` work for `install.sh` too.

## By hand

Download the archive for your platform from the latest release, check it against
`checksums.txt`, and put `claudes` (`claudes.exe` on Windows) in a directory on
`PATH`. Archives cover macOS (`darwin`), Linux, and Windows, on `amd64` and
`arm64`. The binaries are not signed, so one downloaded with a browser is
blocked by Gatekeeper on macOS until you run
`xattr -d com.apple.quarantine claudes`; the install script and `gh` are not.
[Security](security.md) explains how to check a download's provenance.

## Homebrew and winget

With Homebrew, on macOS or Linux, or winget on Windows:

```bash
brew install uvw/tap/claudes
```

```powershell
winget install uvw.claudes
```

winget does not list claudes yet, since Microsoft reviews a package's first
version by hand. Until it does, use the install script.

## With Go

With Go 1.21 or later, `go install` builds the latest release into
`$(go env GOPATH)/bin`, `~/go/bin` by default, and fetches the Go version
`go.mod` names if yours is older:

```bash
go install github.com/uvw/claudes/cmd/claudes@latest
```

## Upgrading

To upgrade a release you installed, run `claudes upgrade`. It downloads the
latest release for your computer, checks it as the install script does, and puts
it in place of the running binary. `claudes upgrade --check` only says whether a
newer release is out, and `--to v0.1.0` installs a given release, older ones
included. A claudes installed with Homebrew, winget, or `go install` prints the
command that upgrades it instead.

Then run `claudes doctor`.

## Uninstalling

1. Remove the binary: `brew uninstall claudes`, `winget uninstall uvw.claudes`,
   or delete `claudes` from where the install script or `go install` put it.
2. Remove the agent skill: delete `~/.claude/skills/claudes`, or the `claudes`
   directory under the `--dir` you gave. A copy added with
   `npx skills add uvw/claudes` goes with `npx skills remove claudes`.
3. Remove the lines [Shell setup](#shell-setup) added to your shell's profile,
   and the zsh completion file, `~/.zfunc/_claudes`.
4. Delete the directories in [Files](reference.md#files) if you want the
   settings and the park list gone too.

## Shell setup

Tab completion offers commands, flags, and the session names each command can
act on: `unpin` offers pinned sessions, `resume` parked and stopped ones, and
`gc` finished ones. `claudes init` adds a shell function so `claudes cd NAME`
changes your shell's directory, which no program can do on its own.

zsh: write the completion script once, to a directory on `fpath` you own, then
add the rest to `~/.zshrc`, before `compinit` runs:

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
as `plan*` against the files in the current directory before claudes sees it.

bash, in `~/.bashrc`. Completion needs the bash-completion package; on macOS's
bash 3.2, use `eval "$(claudes completion bash)"` instead of `source`:

```bash
source <(claudes completion bash)
eval "$(claudes init bash)"
```

fish, in `~/.config/fish/config.fish`:

```fish
claudes completion fish | source
claudes init fish | source
```

PowerShell, in `$PROFILE`:

```powershell
if (Get-Command claudes -ErrorAction SilentlyContinue) {
    Invoke-Expression (& { (claudes completion powershell | Out-String) })
    Invoke-Expression (& { (claudes init pwsh | Out-String) })
}
```

## Agent skill

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
`PATH/claudes/SKILL.md`, for another agent's skills directory.
