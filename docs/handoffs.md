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
docs/decisions.md, and docs/handoffs.md first, and docs/changelog.md for what
the latest release holds. Changes go through pull requests.

Next, in order:
1. Pick up the items in docs/handoffs.md that this machine can do.
2. Work on the open GitHub issues.
3. Ask the owner before any command that changes their live sessions.
```

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
