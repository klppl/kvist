import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { attachmentAllowed, evaluate } from "../src/rules";
import { noteURL } from "../src/slug";

// The same fixtures run in the Go tests (internal/publish, internal/model).
const rules = JSON.parse(readFileSync("../testdata/parity/rules.json", "utf8"));
const urls = JSON.parse(readFileSync("../testdata/parity/urls.json", "utf8"));

test("publish rules match the server", () => {
  for (const c of rules.cases) {
    const d = evaluate(rules.rules, c.path, { frontmatter: c.frontmatter, tags: c.tags ?? [] });
    assert.deepEqual(d, { published: c.published, reason: c.reason }, JSON.stringify(c));
  }
  for (const a of rules.attachments) {
    assert.equal(attachmentAllowed(rules.rules, a.path), a.allowed, a.path);
  }
});

test("published URLs match the server", () => {
  for (const c of urls) {
    assert.equal(noteURL(c.path.normalize("NFC"), c.permalink), c.url, c.path);
  }
});
