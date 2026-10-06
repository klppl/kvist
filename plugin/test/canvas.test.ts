import { test } from "node:test";
import assert from "node:assert/strict";
import { textLinks } from "../src/canvas";

test("textLinks skips links in code blocks and comments, and finds the ones after them", () => {
  const md = [
    "[[Before]]",
    "```js",
    "[[InFence]]",
    "still code ![[in-fence.png]]",
    "```",
    "![[after.png]] and `[[InSpan]]`",
    "~~~~",
    "[[InTilde]]",
    "~~~", // too short to close a ~~~~ fence
    "[[StillInTilde]]",
    "~~~~",
    "%% [[Hidden]] %% <!-- [[AlsoHidden]] -->",
    "[[Last]]",
    "```",
    "[[Unclosed]]",
  ].join("\n");
  assert.deepEqual(textLinks(md).map((l) => l.link), ["Before", "after.png", "Last"]);
});
