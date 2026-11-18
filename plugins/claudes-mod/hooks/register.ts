import type { EngineInterface, Register } from "claude-code";

// claudes writes <state>/mod/requests/<session id>.json for one session:
//
//   { "protocol": 1, "session": "<id>", "expires": <ms>,
//     "requests": [{ "id": "<hex>", "command": "mcp", "args": "disable x" }] }
//
// and waits for <state>/mod/results/<request id>.json. <state> is claudes'
// state directory, found the way claudes finds it. The mod API cannot delete
// a file, so claudes removes the request once it has the results, and a
// request with a result is never run again.

const PROTOCOL = 1;
const POLL_MS = 1500;
const REQUEST_ID = /^[0-9a-f]{16,64}$/;
const SERVER = /^[A-Za-z0-9][A-Za-z0-9_.:@-]{0,199}$/;

export type Request = { id: string; command: string; args: string };

type RequestFile = {
  protocol: number;
  session: string;
  expires: number;
  requests: Request[];
};

type Status = "done" | "refused" | "error";

type Mod = {
  root: string;
  sep: string;
  version: string;
  // mainTurn is the main loop's turn in progress, or null between turns.
  mainTurn: string | null;
  started: boolean;
  busy: boolean;
  announced: string;
  handled: Set<string>;
};

const mod: Mod = {
  root: "",
  sep: "/",
  version: "",
  mainTurn: null,
  started: false,
  busy: false,
  announced: "",
  handled: new Set(),
};

/** Says why the mod refuses to run r, or undefined when r is allowed. */
export function refusal(r: Request): string | undefined {
  if (r.command !== "mcp") return `the mod runs only /mcp, not /${r.command}`;
  const [verb, name, ...rest] = r.args.split(" ");
  if ((verb !== "enable" && verb !== "disable") || rest.length > 0) {
    return "the mod runs only /mcp enable NAME and /mcp disable NAME";
  }
  if (name === undefined || !SERVER.test(name)) return "not a server name";
  return undefined;
}

function parse(text: string): RequestFile | undefined {
  try {
    const f = JSON.parse(text) as Partial<RequestFile>;
    if (
      typeof f.protocol !== "number" ||
      typeof f.session !== "string" ||
      typeof f.expires !== "number" ||
      !Array.isArray(f.requests)
    ) {
      return undefined;
    }
    const requests = f.requests.filter(
      (r): r is Request =>
        typeof r?.id === "string" &&
        REQUEST_ID.test(r.id) &&
        typeof r.command === "string" &&
        typeof r.args === "string",
    );
    return {
      protocol: f.protocol,
      session: f.session,
      expires: f.expires,
      requests,
    };
  } catch {
    return undefined;
  }
}

function path(...parts: string[]): string {
  return [mod.root, ...parts].join(mod.sep);
}

/** Finds claudes' state directory as internal/paths in claudes does. */
async function stateDir(
  $: EngineInterface,
): Promise<{ dir: string; sep: string }> {
  const { platform } = await $.session.info();
  const sep = platform === "windows" ? "\\" : "/";
  const xdg = await $.env.get("XDG_STATE_HOME");
  if (xdg !== undefined && /^(\/|[A-Za-z]:[\\/])/.test(xdg)) {
    return { dir: `${xdg.replace(/[\\/]+$/, "")}${sep}claudes`, sep };
  }
  if (platform === "windows") {
    const local = await $.env.get("LOCALAPPDATA");
    if (local !== undefined && local !== "")
      return { dir: `${local}\\claudes\\state`, sep };
  }
  const home =
    (await $.env.get("HOME")) ?? (await $.env.get("USERPROFILE")) ?? "";
  return { dir: `${home}${sep}.local${sep}state${sep}claudes`, sep };
}

async function writeJSON(
  $: EngineInterface,
  file: string,
  value: unknown,
): Promise<void> {
  await $.fs.write(file, JSON.stringify(value, null, 2) + "\n");
}

async function finish(
  $: EngineInterface,
  sid: string,
  r: Request,
  status: Status,
  reason?: string,
): Promise<void> {
  mod.handled.add(r.id);
  const at = await $.clock.now();
  const result = {
    id: r.id,
    session: sid,
    status,
    reason,
    at,
    version: mod.version,
  };
  await writeJSON($, path("results", `${r.id}.json`), result);
}

// start runs from whichever event comes first, since a mod loaded by
// /reload-plugins in a running session may not see session.start.
async function start($: EngineInterface): Promise<void> {
  if (mod.started) return;
  mod.started = true;
  const { dir, sep } = await stateDir($);
  mod.sep = sep;
  mod.root = `${dir}${sep}mod`;
  const manifest = await $.fs.read(
    `${$.plugin.root}${sep}.claude-plugin${sep}plugin.json`,
  );
  mod.version = String((JSON.parse(manifest) as { version?: unknown }).version);
  $.clock.every(POLL_MS, () => tick($));
}

async function tick($: EngineInterface): Promise<void> {
  if (mod.busy) return;
  mod.busy = true;
  try {
    await serve($);
  } finally {
    mod.busy = false;
  }
}

async function serve($: EngineInterface): Promise<void> {
  const sid = await $.session.id();
  if (sid !== mod.announced) {
    // Tells claudes that this session loaded the mod, and which version.
    const at = await $.clock.now();
    const presence = {
      protocol: PROTOCOL,
      session: sid,
      version: mod.version,
      at,
    };
    await writeJSON($, path("sessions", `${sid}.json`), presence);
    mod.announced = sid;
  }
  const requestFile = path("requests", `${sid}.json`);
  if (!(await $.fs.exists(requestFile))) return;
  const file = parse(await $.fs.read(requestFile));
  if (file === undefined) return;
  const pending: Request[] = [];
  for (const r of file.requests) {
    const seen =
      mod.handled.has(r.id) ||
      (await $.fs.exists(path("results", `${r.id}.json`)));
    if (!seen) pending.push(r);
  }
  if (pending.length === 0) return;

  let reason: string | undefined;
  if (file.protocol !== PROTOCOL)
    reason = `the mod speaks protocol ${PROTOCOL}, not ${file.protocol}`;
  else if (file.session !== sid) reason = "the request names another session";
  else if ((await $.clock.now()) > file.expires) reason = "the request expired";
  if (reason !== undefined) {
    for (const r of pending) await finish($, sid, r, "refused", reason);
    return;
  }

  // Claude Code would queue the command behind a turn itself, but a queued
  // command cannot be taken back, so the mod waits for an idle session.
  const box = await $.prompt.read();
  if (mod.mainTurn !== null || box.text.trim() !== "") return;

  for (const r of pending) {
    const refused = refusal(r);
    if (refused !== undefined) {
      await finish($, sid, r, "refused", refused);
      continue;
    }
    try {
      await $.command.run({ command: r.command, args: r.args });
      await finish($, sid, r, "done");
    } catch (error) {
      await finish(
        $,
        sid,
        r,
        "error",
        error instanceof Error ? error.message : String(error),
      );
    }
  }
}

export const register: Register = (on) => {
  on("turn.start", async ($, e, next) => {
    if (e.agentId === undefined) mod.mainTurn = e.turnId;
    await start($);
    return next(e);
  });
  on("turn.complete", async ($, e, next) => {
    if (e.agentId === undefined) mod.mainTurn = null;
    await start($);
    return next(e);
  });
  on("session.start", async ($, e, next) => {
    const result = await next(e);
    await start($);
    return result;
  });
};
