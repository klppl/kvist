// Publish rules (gate 1). The server is the source of truth for the rules
// and applies them again on its own (internal/publish in the Go server);
// this must decide the same way.

import { Rules, SETTINGS_NOTE_NAME } from "./protocol";

export type Reason =
  | "excluded_folder"
  | "frontmatter_false"
  | "private_tag"
  | "public_folder"
  | "public_tag"
  | "frontmatter_true"
  | "no_rule"
  | "not_a_note"
  | "settings_note";

export interface Decision {
  published: boolean;
  reason: Reason;
}

export interface NoteFacts {
  /** Frontmatter as parsed by Obsidian; undefined if absent. */
  frontmatter?: Record<string, unknown>;
  /** Tags with or without '#', from frontmatter and body (getAllTags). */
  tags: string[];
}

export function isNote(path: string): boolean {
  return path.toLowerCase().endsWith(".md");
}

/** Reports whether path is a settings note (_site.md, any case, any folder). */
export function isSettingsNote(path: string): boolean {
  return path.slice(path.lastIndexOf("/") + 1).toLowerCase() === SETTINGS_NOTE_NAME;
}

export function extension(path: string): string {
  const name = path.slice(path.lastIndexOf("/") + 1);
  const dot = name.lastIndexOf(".");
  return dot <= 0 ? "" : name.slice(dot + 1).toLowerCase();
}

function inFolder(path: string, folder: string, foldCase: boolean): boolean {
  if (folder === "/") return true;
  let f = folder.replace(/^\/+|\/+$/g, "");
  if (!f) return false;
  let p = path;
  if (foldCase) {
    p = p.toLowerCase();
    f = f.toLowerCase();
  }
  return p.startsWith(f + "/");
}

function hasTag(tags: string[], tag: string): boolean {
  if (!tag) return false;
  const want = tag.replace(/^#/, "").toLowerCase();
  return tags.some((t) => t.replace(/^#/, "").toLowerCase() === want);
}

type Flag = "unset" | "true" | "false";

/** Reads the publish property; the key matches case-insensitively and false wins. */
function frontmatterFlag(fm: Record<string, unknown> | undefined, key: string): Flag {
  if (!fm || !key) return "unset";
  let out: Flag = "unset";
  for (const [k, v] of Object.entries(fm)) {
    if (k.toLowerCase() !== key.toLowerCase()) continue;
    let f: Flag = "unset";
    if (v === true || (typeof v === "string" && v.trim().toLowerCase() === "true")) f = "true";
    if (v === false || (typeof v === "string" && v.trim().toLowerCase() === "false")) f = "false";
    if (f === "false") return "false";
    if (f === "true") out = "true";
  }
  return out;
}

/** Evaluate applies the rules to one note. First match wins. */
export function evaluate(rules: Rules, path: string, facts: NoteFacts): Decision {
  if (!isNote(path)) return { published: false, reason: "not_a_note" };
  if (isSettingsNote(path)) return { published: false, reason: "settings_note" };
  for (const f of rules.exclude_folders) {
    if (inFolder(path, f, true)) return { published: false, reason: "excluded_folder" };
  }
  const flag = frontmatterFlag(facts.frontmatter, rules.frontmatter_key);
  if (flag === "false") return { published: false, reason: "frontmatter_false" };
  if (hasTag(facts.tags, rules.private_tag)) return { published: false, reason: "private_tag" };
  for (const f of rules.always_public_folders) {
    if (inFolder(path, f, false)) return { published: true, reason: "public_folder" };
  }
  if (hasTag(facts.tags, rules.public_tag)) return { published: true, reason: "public_tag" };
  if (flag === "true") return { published: true, reason: "frontmatter_true" };
  return { published: false, reason: "no_rule" };
}

/** Attachments are never published from an excluded folder. */
export function attachmentAllowed(rules: Rules, path: string): boolean {
  return !rules.exclude_folders.some((f) => inFolder(path, f, true));
}

/** AllowedPath mirrors the server's manifest check. */
export function allowedPath(rules: Rules, path: string): boolean {
  if (path.split("/").some((seg) => seg.startsWith("."))) return false;
  if (isNote(path)) return true;
  const ext = extension(path);
  return rules.attachment_extensions.some((a) => a.replace(/^\./, "").toLowerCase() === ext);
}

// Image properties name a picture in the frontmatter: a note's social
// preview image, or the site's default image and avatar in the settings
// note. The image they point to is published like an embedded one. Each
// list is one purpose; the first key holding an image wins. Mirrors
// internal/vault/image.go.
export const NOTE_IMAGE_KEYS = ["image", "cover"];
export const SITE_IMAGE_KEYS = ["image"];
export const SITE_AVATAR_KEYS = ["avatar", "logo"];

export type ImageRef = { target: string } | { url: string };

/** Interprets one image property value: a link, a vault path or an http(s) URL. */
export function parseImageValue(v: unknown): ImageRef | null {
  if (Array.isArray(v)) {
    if (v.length === 0) return null;
    v = v[0];
  }
  if (typeof v !== "string") return null;
  let s = v.trim();
  if (s.startsWith("!")) s = s.slice(1);
  if (s.startsWith("[[")) {
    if (!s.endsWith("]]")) return null;
    const target = s.slice(2, -2).split("|")[0].split("#")[0].trim();
    return target ? { target } : null;
  }
  if (isExternal(s)) return /^https?:\/\//i.test(s) ? { url: s } : null;
  if (s === "" || /[[\]\n]/.test(s)) return null;
  return { target: s.split("#")[0].trim() };
}

function isExternal(dest: string): boolean {
  if (dest.startsWith("//")) return true;
  const colon = dest.indexOf(":");
  return colon > 0 && /^[a-zA-Z0-9+.-]+$/.test(dest.slice(0, colon));
}

/** The first of keys (case-insensitive) whose value is an image. */
export function imageProperty(frontmatter: Record<string, unknown> | undefined, keys: string[]): ImageRef | null {
  if (!frontmatter) return null;
  const names = Object.keys(frontmatter).sort();
  for (const key of keys) {
    for (const k of names) {
      if (k.toLowerCase() !== key) continue;
      const r = parseImageValue(frontmatter[k]);
      if (r) return r;
    }
  }
  return null;
}

const IMAGE_EXTENSIONS = new Set(["png", "jpg", "jpeg", "gif", "webp", "svg", "avif", "bmp"]);

export function isImage(path: string): boolean {
  return IMAGE_EXTENSIONS.has(extension(path));
}
