# Changelog

## 0.7.0 (2026-11-20)

### Features

- **cli:** restore what session gc moved to the trash

### Bug fixes

- **cli:** forget a moved session's records when session gc removes it
- four bugs fuzzing found in search and printed commands, and weekly fuzz tests

## 0.6.0 (2026-11-18)

### Features

- **cli:** filter session ls by running state, and take today, yesterday, week,
  and month for durations

### Bug fixes

- **cli:** cut long branches and fit names to the terminal in every table
- **cli:** show session ls's branch column only with --wide

## 0.5.0 (2026-11-18)

### Features

- **cli:** add short forms for the flags typed most often
- **cli:** carry a stopped background job over in session mv
- **cli:** list the MCP servers a session or directory has with claudes mcp ls
- **cli:** mask secrets in session search and rename --limit to --top
- **cli:** search past sessions by what was said in them
- **cli:** shorten long directories to fit every text table
- **cli:** show running subagents in ls, and more with --wide
- **cli:** warn in doctor when Claude Code may write pins without the lock
- **procs:** read process start times from /proc on linux
- **safefile:** replace open files with posix rename semantics on windows

### Bug fixes

- **claude:** drop the child-notes and effort variables when starting claude
- **cli:** add --confirm to gc and session gc, and lay out cli by command
- **cli:** close safety gaps in gc, session mv, pause, and their hints
- **cli:** trash what gc --teams and session mv --rm remove, and survive SIGTERM
  and a suspend
- **deps:** Bump charm.land/lipgloss/v2 from 2.0.6 to 2.1.0 in the gomod group
  across 1 directory
- **hack:** date the names of the release tokens so a renewal can be made
- judge sessions only on records claudes knows, and harden moves and renames
- **procs:** bound the start time read from /proc
- **procs:** refuse a process that started after its session
- **procs:** stop an npm-installed claude code on windows
- **style:** follow the clicolors convention, and check Claude Code's formats
  weekly

## 0.4.0 (2026-11-17)

### Features

- **cli:** give every table the same column order and cut long names
- **cli:** list the biggest sessions with session du --top
- **cli:** remove sessions that did nothing with session gc --trivial
- **cli:** sort past sessions into remove, review, and keep with session triage
- **cli:** trim saved outputs and old subagents with session gc --sidecars
- say directory instead of folder everywhere

## 0.3.0 (2026-11-16)

### Features

- **cli:** take pinned sessions in bulk with --pinned

### Bug fixes

- **cli:** read a running session's version from the binary it runs

## 0.2.1 (2026-11-16)

### Features

- **cli:** answer to t and teams for claudes team

### Bug fixes

- **safefile:** wait for a lock that another process is removing on windows

## 0.2.0 (2026-11-16)

### Features

- **cli:** add claudes init fish, so claudes cd works in fish
- **cli:** answer to s and sessions for claudes session
- **cli:** remove empty, copied, and leftover sessions with session gc
- **cli:** show how much disk space sessions take
- **cli:** upgrade claudes in place with claudes upgrade
- **release:** install claudes with one command on macos, linux, and windows
- **release:** publish each release to homebrew and winget

### Bug fixes

- **e2e:** report a failed close of the fake's call log
- **release:** find the release pull request when formatting its changelog

## 0.1.0 (2026-11-15)

### Features

- add paths, atomic files, quoting, process control, and matching
- check Claude Code compatibility, and read transcripts and agent teams
- **claude:** reach the claude command line through one client
- **cli:** add the claudes commands and the agent skill
- read and write agent view's jobs and pins, the park file, and config.yaml

### Bug fixes

- **cli:** keep --verbose from passing through, and report refusals and failures
  in full
- **cli:** match sessions whose folder was recorded through a link
