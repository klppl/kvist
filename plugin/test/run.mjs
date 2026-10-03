// Bundles test/*.test.ts with esbuild and runs them with node --test.
import esbuild from "esbuild";
import { readdirSync, rmSync } from "node:fs";
import { spawnSync } from "node:child_process";

const tests = readdirSync("test").filter((f) => f.endsWith(".test.ts"));
rmSync(".test-build", { recursive: true, force: true });
await esbuild.build({
  entryPoints: tests.map((f) => "test/" + f),
  bundle: true,
  platform: "node",
  format: "esm",
  target: "node20",
  outdir: ".test-build",
  outExtension: { ".js": ".mjs" },
  logLevel: "warning",
});
const files = tests.map((f) => ".test-build/" + f.replace(/\.ts$/, ".mjs"));
const r = spawnSync(process.execPath, ["--test", ...files], { stdio: "inherit" });
process.exit(r.status ?? 1);
