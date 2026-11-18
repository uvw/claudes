## TL;DR

- **Fixed:** `gc`, `session gc`, `session mv`, and `pause` no longer remove or
  skip the wrong thing (#89, #90).
- **Changed:** `gc --teams` and `session mv --rm` move files to the trash, and
  `--purge` deletes for good (#95).
- **New:** `claudes session search` finds past sessions by what was said in them
  (#78).
- **New:** `claudes mcp ls` lists the MCP servers a session or directory has
  (#81).
- **Changed:** `ls` shows running subagents, and long directories shrink to fit
  tables (#74, #85).

This release makes the cleanup commands harder to misfire. It also adds ways to
find old sessions and see what a session runs.

## Behavior changes

### Safety

1. `gc --teams` keeps the team and task list of a stopped pinned session too,
   unless you pass `--pinned`.
2. `gc` and `session gc` take `--confirm` (`-c`) and ask y/N before they remove
   anything (#92).
3. `session gc` keeps a session whose transcript has a record type claudes does
   not know, instead of trashing it.
4. `session mv` and `session resume --bg` now run the compatibility check.
   `session resume` takes `--force`.
5. claudes refuses to stop a process that started after its session (#70).
6. On Unix, SIGTERM and SIGHUP end a run like Ctrl-C, so you still get the
   report. A suspended claudes keeps its run lock.
7. `doctor` warns when Claude Code may write `pins.json` without the lock (#66).

### Sessions

1. `session mv` of a stopped background job removes the job and keeps its
   options. `session resume --bg` then starts it in the new directory with them.
2. `session search QUERY...` (alias `grep`) ranks sessions, best first. It masks
   secrets as `[redacted]`. `--no-redact` prints them. `--top N` (`-t`) sets how
   many to show.

### Listing

1. `ls` replaces `TEAM` with `AGENTS`, which shows a team you lead and subagents
   running now. `ls --wide` adds `SHELLS`, `TOKENS`, `PRS`, and `DOING`.
2. `mcp ls [TARGET] [-i DIR] [--profile NAME]` lists servers by scope and state.
   `mcp profiles` lists the profiles in `config.yaml`.
3. Every text table shortens a directory that does not fit. Set `output.dirs` to
   `middle`, `fish`, or `full`.
4. Short flags: `-u` for `restart --outdated`, `-i` for `--in`, `-l` for
   `--since`, `-t` for `--tail` or `--top`, `-b` for `session resume --bg`, and
   `-r` for `session mv --resume`.
5. `CLICOLOR=1` no longer colors piped output. `CLICOLOR_FORCE=1` does.

## Scripts that read claudes' output

1. `ls -o json` keeps `teammates` and adds `subagents`, `shells`, `tokens`,
   `prs`, and `doing`. Read the `AGENTS` column, not `TEAM`.
2. `where` and `session where` print several matches as a table with a header
   row. Use `--one` or `-o json` instead.
3. `logs LEAD/UNKNOWN` and `session where --one` with several matches now exit
   2, not 1.
4. `session gc -o json` reports `session gc` as its command. Older copies have
   an empty `id`.
5. `doctor` and `doctor env` follow `output.format`.

## Platform notes

1. Windows: `stop` and `--interactive` accept an npm-installed Claude Code
   (#71).
2. Windows: pin writes no longer wait while agent view holds `pins.json` open
   (#75).
3. Linux: start times come from `/proc`, exact to 10 ms (#67).

The full list of changes follows.
