import { test } from "node:test";
import assert from "node:assert/strict";
import { HashCache } from "../src/hash";
import { Rules } from "../src/protocol";
import { scan } from "../src/scan";
import { MemVault } from "./memvault";

const rules: Rules = {
  always_public_folders: ["Garden"],
  exclude_folders: ["Private"],
  public_tag: "public",
  private_tag: "private",
  frontmatter_key: "publish",
  attachment_extensions: ["png"],
};

test("scan publishes notes, their attachments and safe hints", async () => {
  const v = new MemVault();
  v.write("Garden/Welcome.md", "See [[Leaf]], [[Diary]] and ![[pic.png]] and ![[secret.png]]");
  v.write("Garden/Leaf.md", "leaf");
  v.write("Journal/Diary.md", "SECRET");
  v.write("Garden/pic.png", "PNG");
  v.write("Private/secret.png", "PNG2");
  v.write("unused.png", "PNG3");
  v.write(".kvist/site.toml", "title = 'x'"); // a stray dot file stays home
  const r = await scan(v, rules, new HashCache());
  assert.deepEqual(r.files.map((f) => f.path), [
    ".kvist/links.json", "Garden/Leaf.md", "Garden/Welcome.md", "Garden/pic.png",
  ]);
  const hints = JSON.parse(new TextDecoder().decode(await r.content.get(".kvist/links.json")!()));
  assert.deepEqual(hints, { version: 1, notes: { "Garden/Welcome.md": { Diary: null, Leaf: "Garden/Leaf.md" } } });
  assert.ok(!JSON.stringify(hints).includes("Journal"));
  assert.deepEqual(r.leaks.map((l) => [l.kind, l.target]).sort(), [
    ["excluded_attachment", "Private/secret.png"],
    ["unpublished_link", "Journal/Diary.md"],
  ]);
});

test("scan reuses cached hashes", async () => {
  const v = new MemVault();
  v.write("Garden/a.md", "a", 1000);
  const cache = new HashCache();
  await scan(v, rules, cache);
  const reads = v.reads;
  await scan(v, rules, cache);
  assert.equal(v.reads, reads, "unchanged files were read again");
});

test("scan refuses paths that differ only by case", async () => {
  const v = new MemVault();
  v.write("Garden/A.md", "a");
  v.write("garden/a.md", "b");
  await assert.rejects(scan(v, rules, new HashCache()), /differ only by case/);
});

test("scan sends the settings note as .kvist/site.md and never as a page", async () => {
  const v = new MemVault();
  v.write("Garden/Welcome.md", "hi");
  v.write("Garden/_site.md", "---\ntitle: Mine\n---\n- [About](/about/)");
  const r = await scan(v, rules, new HashCache());
  assert.deepEqual(r.files.map((f) => f.path), [".kvist/site.md", "Garden/Welcome.md"]);
  assert.equal(r.settingsNote, "Garden/_site.md");
  assert.equal(r.decisions.get("Garden/_site.md")?.reason, "settings_note");
  const sent = new TextDecoder().decode(await r.content.get(".kvist/site.md")!());
  assert.match(sent, /title: Mine/);
});

test("scan refuses two settings notes", async () => {
  const v = new MemVault();
  v.write("_site.md", "a");
  v.write("Garden/_site.md", "b");
  await assert.rejects(scan(v, rules, new HashCache()), /2 settings notes \(Garden\/_site.md, _site.md\); keep one/);
});

test("scan publishes images named in properties, never from excluded folders", async () => {
  const v = new MemVault();
  v.write("Garden/Note.md", "---\nimage: [[cover.png]]\n---\nText");
  v.write("Garden/Web.md", "---\ncover: https://example.com/c.png\n---\nText");
  v.write("Garden/Hidden.md", "---\ncover: Private/secret.png\n---\nText");
  v.write("Journal/Diary.md", "---\nimage: [[diary.png]]\n---\nSECRET");
  v.write("Garden/_site.md", "---\navatar: [[me.png]]\nfavicon: [[icon.png]]\nimage: [[Private/og.png]]\n---\n");
  for (const p of ["Garden/cover.png", "Private/secret.png", "diary.png", "me.png", "icon.png", "Private/og.png"]) v.write(p, p);
  const r = await scan(v, rules, new HashCache());
  assert.deepEqual(r.files.map((f) => f.path).filter((p) => p.endsWith(".png")), ["Garden/cover.png", "icon.png", "me.png"]);
  assert.deepEqual(r.leaks.map((l) => [l.from, l.target]).sort(), [
    ["Garden/Hidden.md", "Private/secret.png"],
    ["Garden/_site.md", "Private/og.png"],
  ]);
});
