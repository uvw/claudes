package compat

// PinsFlag is the server-side flag behind a second way Claude Code writes
// pins.json: through its storage layer, which may not take pins.json.lock.
// While it is off, Claude Code takes the lock claudes takes. mcp.Store.Feature
// reads it.
const PinsFlag = "tengu_hover_rest"
