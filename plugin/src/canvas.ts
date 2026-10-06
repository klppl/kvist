// Reads what a canvas (.canvas, JSON Canvas) points to, so the files it
// shows are published with it. Mirrors Canvas.Links in internal/vault: file
// cards embed their file, text cards are Markdown with links and embeds.

import type { LinkInfo } from "./scan";

interface CanvasNode {
  type?: unknown;
  text?: unknown;
  file?: unknown;
  subpath?: unknown;
}

/** Lists the links of a canvas; an unreadable canvas has none. */
export function canvasLinks(json: string): LinkInfo[] {
  let nodes: unknown;
  try {
    nodes = (JSON.parse(json) as { nodes?: unknown })?.nodes;
  } catch {
    return [];
  }
  if (!Array.isArray(nodes)) return [];
  const out: LinkInfo[] = [];
  for (const n of nodes as CanvasNode[]) {
    if (!n || typeof n !== "object") continue;
    if (n.type === "file" && typeof n.file === "string" && n.file.trim()) {
      const sub = typeof n.subpath === "string" ? n.subpath : "";
      // Not a wikilink: the server resolves the exact path, without hints.
      out.push({ link: n.file + (sub && !sub.startsWith("#") ? "#" : "") + sub, original: "", embed: true });
    } else if (n.type === "text" && typeof n.text === "string") {
      out.push(...textLinks(n.text));
    }
  }
  return out;
}

/** Links and embeds in Markdown, outside code and comments. */
export function textLinks(md: string): LinkInfo[] {
  const text = md
    .replace(/^(\s*)(```|~~~)[^\n]*\n[\s\S]*?(\n\s*\2[^\n]*|$)/gm, "")
    .replace(/`[^`\n]*`/g, "")
    .replace(/%%[\s\S]*?(%%|$)/g, "")
    .replace(/<!--[\s\S]*?(-->|$)/g, "");
  const out: LinkInfo[] = [];
  for (const m of text.matchAll(/(!?)\[\[([^\]\n]+)\]\]|(!?)\[[^\]\n]*\]\(([^)\n]+)\)/g)) {
    if (m[2] !== undefined) {
      const link = m[2].split("|")[0].trim();
      if (link) out.push({ link, original: m[0], embed: m[1] === "!" });
      continue;
    }
    let dest = m[4].trim();
    if (dest.startsWith("<")) dest = dest.slice(1, dest.indexOf(">") > 0 ? dest.indexOf(">") : undefined);
    else dest = dest.split(/\s+/)[0];
    if (!dest || dest.startsWith("#") || dest.startsWith("//") || /^[a-zA-Z][a-zA-Z0-9+.-]*:/.test(dest)) continue;
    try {
      dest = decodeURIComponent(dest);
    } catch {
      // keep it as written
    }
    out.push({ link: dest, original: m[0], embed: m[3] === "!" });
  }
  return out;
}
