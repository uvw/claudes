import type { On } from "claude-code";
import { expect, mock, test } from "claude-code/testing";

const SID = "0b7c3f9e-1111-2222-3333-444455556666";
const ROOT = "/home/me/.local/state/claudes/mod";
const REQUEST = `${ROOT}/requests/${SID}.json`;
const ID = "00112233445566778899aabbccddeeff";

type World = {
  files: Map<string, string>;
  commands: string[];
  box: { text: string };
  // rejects makes every command fail, as an unknown one does.
  rejects: boolean;
};

// world stands for the engine and the disk beneath the mod.
function world(
  on: On,
  env: Record<string, string> = { HOME: "/home/me" },
): World {
  const w: World = {
    files: new Map(),
    commands: [],
    box: { text: "" },
    rejects: false,
  };
  mock.env(on, env);
  on("session.info", () => ({
    value: { transcriptPath: "", platform: "macos" },
  }));
  on("session.id", () => ({ value: SID }));
  on("session.start", (_, e) => ({ cwd: e.cwd }));
  on("fs.exists", (_, e) => ({ value: w.files.has(e.path) }));
  on("fs.read", (_, e) => {
    if (e.path.endsWith("/.claude-plugin/plugin.json"))
      return { value: '{ "version": "9.9.9" }' };
    const text = w.files.get(e.path);
    if (text === undefined) throw new Error(`ENOENT: ${e.path}`);
    return { value: text };
  });
  on("fs.write", (_, e) => {
    w.files.set(e.path, e.text);
    return { value: undefined };
  });
  on("prompt.read", () => ({ value: { text: w.box.text, cursor: 0 } }));
  on("command.run", (_, e) => {
    if (w.rejects) throw new Error("no such server");
    w.commands.push(`/${e.command} ${e.args}`);
    return {};
  });
  return w;
}

function request(
  w: World,
  requests: unknown[],
  extra: Record<string, unknown> = {},
): void {
  const file = {
    protocol: 1,
    session: SID,
    expires: 60_000,
    requests,
    ...extra,
  };
  w.files.set(REQUEST, JSON.stringify(file));
}

function result(w: World, id = ID): Record<string, unknown> | undefined {
  const text = w.files.get(`${ROOT}/results/${id}.json`);
  return text === undefined
    ? undefined
    : (JSON.parse(text) as Record<string, unknown>);
}

const START = {
  cwd: "/work",
  surface: "terminal",
  isInteractive: true,
} as const;

test("runs an allowed /mcp request once the session is idle, and only once", async ($, on) => {
  const clock = mock.clock(on, { now: 1_000 });
  const w = world(on);
  await $.session.start(START);
  request(w, [{ id: ID, command: "mcp", args: "disable browser" }]);

  await clock.advance(1_500);
  expect(w.commands).toEqual(["/mcp disable browser"]);
  expect(result(w)).toEqual({
    id: ID,
    session: SID,
    status: "done",
    at: 2_500,
    version: "9.9.9",
  });

  await clock.advance(3_000);
  expect(w.commands).toEqual(["/mcp disable browser"]);
});

test("says the session loaded it, with its version", async ($, on) => {
  const clock = mock.clock(on);
  const w = world(on);
  await $.session.start(START);
  await clock.advance(1_500);
  const presence = JSON.parse(
    w.files.get(`${ROOT}/sessions/${SID}.json`) ?? "{}",
  );
  expect(presence).toEqual({
    protocol: 1,
    session: SID,
    version: "9.9.9",
    at: 1_500,
  });
});

test("waits while a turn runs or the prompt box holds a draft", async ($, on) => {
  const clock = mock.clock(on);
  const w = world(on);
  on("turn.start", (_, e) => ({ turnId: e.turnId }));
  on("turn.complete", () => ({ text: "" }));
  await $.session.start(START);
  request(w, [{ id: ID, command: "mcp", args: "enable browser" }]);

  await $.turn.start({ text: "hi", turnId: "t1" });
  await clock.advance(3_000);
  expect(w.commands).toEqual([]);

  await $.turn.complete({ turnId: "t1", reason: "answer" } as never);
  w.box.text = "half a thought";
  await clock.advance(3_000);
  expect(w.commands).toEqual([]);

  w.box.text = "";
  await clock.advance(1_500);
  expect(w.commands).toEqual(["/mcp enable browser"]);
});

test("refuses anything but /mcp enable or disable of one server", async ($, on) => {
  const clock = mock.clock(on);
  const w = world(on);
  await $.session.start(START);
  const ids = ["a", "b", "c", "d"].map((c) => c.repeat(16));
  request(w, [
    { id: ids[0], command: "compact", args: "" },
    { id: ids[1], command: "mcp", args: "add evil" },
    { id: ids[2], command: "mcp", args: "disable two servers" },
    { id: ids[3], command: "mcp", args: "disable $(rm)" },
  ]);

  await clock.advance(1_500);
  expect(w.commands).toEqual([]);
  for (const id of ids) expect(result(w, id)?.status).toBe("refused");
});

for (const [what, extra] of [
  ["an expired request", { expires: 0 }],
  ["another session's request", { session: "someone-else" }],
  ["another protocol's request", { protocol: 2 }],
] as const) {
  test(`refuses ${what}`, async ($, on) => {
    const clock = mock.clock(on, { now: 10 });
    const w = world(on);
    await $.session.start(START);
    request(w, [{ id: ID, command: "mcp", args: "disable browser" }], extra);
    await clock.advance(1_500);
    expect(w.commands).toEqual([]);
    expect(result(w)?.status).toBe("refused");
  });
}

test("skips a request id that is not hex, since it names the result file", async ($, on) => {
  const clock = mock.clock(on);
  const w = world(on);
  await $.session.start(START);
  request(w, [{ id: "../../escape", command: "mcp", args: "disable browser" }]);
  await clock.advance(1_500);
  expect(w.commands).toEqual([]);
  expect([...w.files.keys()].some((p) => p.includes("escape"))).toBe(false);
});

test("reports an error when the command is rejected", async ($, on) => {
  const clock = mock.clock(on);
  const w = world(on);
  w.rejects = true;
  await $.session.start(START);
  request(w, [{ id: ID, command: "mcp", args: "disable browser" }]);
  await clock.advance(1_500);
  expect(result(w)?.status).toBe("error");
});

test("finds the state directory through XDG_STATE_HOME", async ($, on) => {
  const clock = mock.clock(on);
  const w = world(on, { HOME: "/home/me", XDG_STATE_HOME: "/xdg/" });
  await $.session.start(START);
  await clock.advance(1_500);
  expect(w.files.has(`/xdg/claudes/mod/sessions/${SID}.json`)).toBe(true);
});
