// Reads what a canvas (.canvas, JSON Canvas) points to, so the files it
// shows are published with it. Mirrors Canvas.Links in internal/vault: file
// cards embed their file, text cards are Markdown with links and embeds.

import { stableJSON, type LinkInfo } from "./scan";

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

/**
 * Removes the file cards of a canvas whose file is not going up, and the
 * arrows to and from them, so a canvas never carries the vault paths of
 * private notes or files to the server. Mirrors stripCanvas in
 * internal/pushclient: a canvas with nothing to remove comes back as it is,
 * otherwise it is re-encoded with sorted keys and no spaces. Returns null
 * when the canvas isn't a JSON object.
 */
export function stripCanvas(json: string, keep: (file: string) => boolean): string | null {
  let doc: unknown;
  try {
    doc = JSON.parse(json);
  } catch {
    return null;
  }
  if (!doc || typeof doc !== "object" || Array.isArray(doc)) return null;
  const d = doc as Record<string, unknown>;
  const nodes = Array.isArray(d.nodes) ? d.nodes : [];
  const dropped = new Set<string>();
  const kept = nodes.filter((n) => {
    const c = n as CanvasNode & { id?: unknown };
    if (!c || typeof c !== "object" || c.type !== "file" || typeof c.file !== "string" || !c.file.trim() || keep(c.file)) {
      return true;
    }
    if (typeof c.id === "string") dropped.add(c.id);
    return false;
  });
  if (kept.length === nodes.length) return json;
  d.nodes = kept;
  if (Array.isArray(d.edges)) {
    d.edges = d.edges.filter((e) => {
      const x = e as { fromNode?: unknown; toNode?: unknown };
      return !x || typeof x !== "object" || !(dropped.has(x.fromNode as string) || dropped.has(x.toNode as string));
    });
  }
  return stableJSON(d);
}
