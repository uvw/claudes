# Handoffs

Work one agent or machine left for another to pick up. Each item sits under the
platform that can do it.

Every item has:

- **What:** the change or check, with the commits or files involved.
- **Verify:** how to tell it works, step by step.
- **Added:** the date and the session or platform that left it.

When an item is done, delete it here and say what you found in the commit
message. If you find a bug, fix it in the same change or leave a new item.

## Resume prompt

Paste this into a new Claude Code session at the root of this repository:

```text
Continue work on claudes. Read AGENTS.md, docs/contributing.md,
docs/decisions.md, and docs/handoffs.md first.

State: 0.1.0 is released. Every command in the README is built and tested.
Every command under "Checks" in docs/contributing.md passes on Windows and
Linux, CI passes on Linux, macOS, and Windows, and every command has run
against throwaway sessions on Claude Code 2.1.325. Changes go through pull
requests.

Next, in order:
1. Pick up the items in docs/handoffs.md that this machine can do.
2. Work on the open GitHub issues.
3. Ask the owner before any command that changes their live sessions.
```

## Windows only

1. **`claudes stop` on an interactive session listed by agent view.**
   - **What:** `claudes stop` finds an interactive session through
     `claude agents --json`, then kills its process tree. The tree kill works on
     a real `claude.exe`, but a session started in a console with no window was
     never listed, so the lookup has only run in tests.
   - **Verify:** start `claude -n stop-test` in a Windows Terminal tab and send
     it one prompt. Run `claudes ls`: it shows `stop-test` as `tty`. Run
     `claudes stop stop-test`: the tab's session ends, with its shells.
   - **Added:** 2026-11-14, Windows session.

2. **`claudes session gc` moving to the Recycle Bin.**
   - **What:** `internal/trash/trash_windows.go` calls `SHFileOperationW` with
     `FOF_ALLOWUNDO`. It compiles and vets, but has never run on Windows.
   - **Verify:** in any project directory under
     `%USERPROFILE%\.claude\projects`, make a scratch directory named by a
     session id that has no transcript, such as
     `00000000-0000-0000-0000-000000000000`, with a file inside. Set the times
     of both back two days. Run `claudes session gc --leftovers --dry-run`: the
     plan lists it. Run it without `--dry-run`: the directory is gone and the
     Recycle Bin holds it, and "Restore" puts it back.
   - **Added:** 2026-11-16, macOS session.

## Linux only

1. **`claudes session gc` moving to the freedesktop.org trash.**
   - **What:** `internal/trash/freedesktop.go` writes `files/` and `info/` under
     `$XDG_DATA_HOME/Trash`. Tests check the layout on macOS; no file manager
     has restored from it yet.
   - **Verify:** as in the Windows item, make a leftover directory, run
     `claudes session gc --leftovers`, then open the trash in the desktop's file
     manager: it lists the directory with its original location, and restoring
     puts it back.
   - **Added:** 2026-11-16, macOS session.
