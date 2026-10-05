// End to end: the plugin's publisher against a real kvist server.
// Builds the server with `go build` (or uses $KVIST_BIN); skipped without Go.
import { test, before, after } from "node:test";
import assert from "node:assert/strict";
import { spawn, spawnSync, ChildProcess } from "node:child_process";
import { mkdtempSync, writeFileSync } from "node:fs";
import { createServer } from "node:net";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { Client, Transport } from "../src/client";
import { HashCache } from "../src/hash";
import { DeviceState, Publisher, StaleCancelled } from "../src/publisher";
import { MemVault } from "./memvault";

let bin = process.env.KVIST_BIN ?? "";
let server: ChildProcess | undefined;
let base = "";
let token = "";
let skip: string | false = false;

const fetchTransport: Transport = async (req) => {
  const res = await fetch(req.url, {
    method: req.method,
    headers: { ...req.headers, ...(req.contentType ? { "Content-Type": req.contentType } : {}) },
    body: req.body as BodyInit | undefined,
  });
  return { status: res.status, text: await res.text() };
};

function freePort(): Promise<number> {
  return new Promise((ok) => {
    const s = createServer().listen(0, "127.0.0.1", () => {
      const port = (s.address() as { port: number }).port;
      s.close(() => ok(port));
    });
  });
}

before(async () => {
  const dir = mkdtempSync(join(tmpdir(), "kvist-e2e-"));
  if (!bin) {
    bin = join(dir, "kvist");
    const r = spawnSync("go", ["build", "-o", bin, "./cmd/kvist"], { cwd: resolve(".."), stdio: "inherit" });
    if (r.error || r.status !== 0) {
      skip = "go is not available";
      return;
    }
  }
  const port = await freePort();
  base = `http://127.0.0.1:${port}`;
  const cfg = join(dir, "kvist.toml");
  writeFileSync(cfg, `data_dir = "${join(dir, "data")}"
listen = "127.0.0.1:${port}"
[[site]]
id = "garden"
base_url = "https://garden.example.com"
root_folder = "/" # keep the folder in addresses
serve = true
  [site.publish]
  always_public_folders = ["Garden"]
  exclude_folders = ["Private"]
`);
  const t = spawnSync(bin, ["token", "create", "--config", cfg, "--site", "garden", "--name", "e2e"], { encoding: "utf8" });
  token = t.stdout.trim();
  server = spawn(bin, ["serve", "--config", cfg], { stdio: "ignore" });
  for (let i = 0; i < 100; i++) {
    try {
      if ((await fetch(base + "/api/v1/info")).ok) return;
    } catch {
      // not up yet
    }
    await new Promise((r) => setTimeout(r, 100));
  }
  throw new Error("server did not start");
});

after(() => server?.kill());

function device(vault: MemVault) {
  const state: DeviceState = { clientId: Math.random().toString(16).slice(2), baseRevision: "" };
  const client = new Client(base, "garden", token, fetchTransport);
  return new Publisher(client, vault, new HashCache(), state, () => {}, "test", "node", "0.1.0");
}

async function page(path: string) {
  const r = await fetch(base + path, { redirect: "manual" });
  return { status: r.status, text: await r.text(), location: r.headers.get("location") };
}

test("publish, rename, unpublish and stale state against the server", async (t) => {
  if (skip) return t.skip(skip);
  const v = new MemVault();
  const t0 = Date.parse("2026-10-01T10:00:00Z");
  v.write("Garden/Hello.md", "Hello [[Second]] and [[Diary]]", t0);
  v.write("Garden/Second.md", "Second note", t0);
  v.write("Journal/Diary.md", "SECRET", t0);
  const laptop = device(v);

  const r1 = await laptop.publish({ waitForBuild: true });
  assert.equal(r1.build?.state, "succeeded");
  assert.equal(r1.uploaded, 3); // two notes and the hints file
  let p = await page("/garden/hello/");
  assert.equal(p.status, 200);
  assert.match(p.text, /<span class="link-unpublished">Diary<\/span>/);
  assert.equal((await page("/journal/diary/")).status, 404);

  // Nothing changed: skipped without a request to /syncs.
  const r2 = await laptop.publish({ skipIfUnchanged: true });
  assert.equal(r2.skipped, true);

  // Rename: no content is uploaded again (only the hints file changes).
  v.rename("Garden/Second.md", "Garden/Renamed.md");
  v.write("Garden/Hello.md", "Hello [[Renamed]] and [[Diary]]", t0 + 1000);
  const r3 = await laptop.publish({ waitForBuild: true });
  assert.ok(r3.uploaded <= 2, `rename uploaded ${r3.uploaded} blobs`);
  assert.equal((await page("/garden/renamed/")).status, 200);
  // The old address redirects to the new one.
  p = await page("/garden/second/");
  assert.equal(p.status, 301);
  assert.equal(p.location, "/garden/renamed/");

  // Unpublish: the page and the redirect to it are gone.
  v.write("Garden/Renamed.md", "---\npublish: false\n---\nnow private", t0 + 2000);
  await laptop.publish({ waitForBuild: true });
  assert.equal((await page("/garden/renamed/")).status, 404);
  assert.equal((await page("/garden/second/")).status, 404);

  // A phone that hasn't synced holds an older Hello: it must ask first.
  const old = new MemVault();
  old.write("Garden/Hello.md", "Old hello", t0 - 60000);
  const phone = device(old);
  let asked = 0;
  await assert.rejects(
    phone.publish({ confirmStale: async (_m, regs) => { asked = regs.length; return false; } }),
    StaleCancelled,
  );
  assert.ok(asked >= 1);
  p = await page("/garden/hello/");
  assert.match(p.text, /Hello/);
  assert.doesNotMatch(p.text, /Old hello/);
});
