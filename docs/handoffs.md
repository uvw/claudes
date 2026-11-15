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
docs/decisions.md, docs/todo.md, and docs/handoffs.md first.

State: every command in the README is built and tested, and every command under
"Checks" in docs/contributing.md passes on Windows and Linux, and CI passes on
Linux, macOS, and Windows. Changes go through pull requests. Nothing has run
against live sessions except read-only commands and --dry-run.

Next, in order:
1. Pick up the items in docs/handoffs.md that this machine can do.
2. Work through docs/todo.md with the owner.
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

## Up for grabs

1. **A real run with the owner.**
   - **What:** "Before the first release" in `docs/todo.md`.
   - **Verify:** the owner runs `restart`, `pause`, `resume`, and `ping` against
     live sessions without surprises.
   - **Added:** 2026-11-14, Windows session.
