// Builds the manifest from a vault: gate 1 (the publish rules on Obsidian's
// metadata), the attachments that published notes reference, the settings
// note (_site.md, sent as .kvist/site.md), the optional .kvist/site.toml and
// the hints file. Obsidian-independent so it can be
// tested; main.ts adapts the real vault to VaultLike.

import { HashCache, sha256 } from "./hash";
import { HINTS_PATH, ManifestFile, Rules, SITE_CONFIG_PATH, SITE_NOTE_PATH } from "./protocol";
import { Decision, allowedPath, attachmentAllowed, evaluate, isNote, isSettingsNote } from "./rules";

export interface VaultFile {
  path: string;
  mtime: number; // ms since epoch
  size: number;
}

export interface LinkInfo {
  /** Link target as Obsidian stores it: "Note#Heading" (no alias). */
  link: string;
  /** The link as written, e.g. "[[Note#Heading|alias]]". */
  original: string;
  embed: boolean;
}

export interface NoteMeta {
  frontmatter?: Record<string, unknown>;
  tags: string[];
  links: LinkInfo[];
}

export interface VaultLike {
  files(): VaultFile[];
  read(path: string): Promise<ArrayBuffer>;
  /** Reads a file Obsidian doesn't index (e.g. in .kvist/); null if absent. */
  readHidden(path: string): Promise<{ data: ArrayBuffer; mtime: number } | null>;
  meta(path: string): NoteMeta | null;
  /** Resolves a link path from a note to a vault path, or null. */
  resolve(linkpath: string, source: string): string | null;
}

export interface LeakItem {
  kind: "unpublished_link" | "unpublished_embed" | "excluded_attachment";
  from: string;
  /** Vault path of the target. Shown locally, never sent to the server. */
  target: string;
}

export interface ScanResult {
  files: ManifestFile[];
  decisions: Map<string, Decision>;
  leaks: LeakItem[];
  content: Map<string, () => Promise<ArrayBuffer>>;
  /** Vault path of the settings note (_site.md), if there is one. */
  settingsNote?: string;
}

/** Finds the settings note; throws if there are several. */
export function findSettingsNote(paths: Iterable<string>): string | undefined {
  const found = [...paths].filter(isSettingsNote).sort();
  if (found.length > 1) throw new Error(`found ${found.length} settings notes (${found.join(", ")}); keep one`);
  return found[0];
}

/** Splits "Note#Heading" into the target part, as the server keys hints. */
function linkTarget(link: string): string {
  const i = link.indexOf("#");
  return (i >= 0 ? link.slice(0, i) : link).trim();
}

function isWikilink(original: string): boolean {
  return original.startsWith("[[") || original.startsWith("![[");
}

const iso = (ms: number) => new Date(Math.floor(ms)).toISOString();

export async function scan(vault: VaultLike, rules: Rules, cache: HashCache): Promise<ScanResult> {
  const all = new Map<string, VaultFile>();
  const folded = new Map<string, string>();
  for (const f of vault.files()) {
    const p = f.path.normalize("NFC");
    const k = p.toLowerCase();
    const other = folded.get(k);
    if (other !== undefined && other !== p) {
      throw new Error(`"${other}" and "${p}" differ only by case; rename one`);
    }
    folded.set(k, p);
    all.set(p, { ...f, path: p });
  }

  const decisions = new Map<string, Decision>();
  const published = new Set<string>();
  for (const f of all.values()) {
    if (!isNote(f.path)) continue;
    const m = vault.meta(f.path);
    const d = evaluate(rules, f.path, { frontmatter: m?.frontmatter, tags: m?.tags ?? [] });
    decisions.set(f.path, d);
    if (d.published) published.add(f.path);
  }

  const include = new Set<string>(published);
  const leaks: LeakItem[] = [];
  const hints: Record<string, Record<string, string | null>> = {};
  for (const from of [...published].sort()) {
    for (const l of vault.meta(from)?.links ?? []) {
      const target = linkTarget(l.link);
      if (!target) continue;
      const dest = vault.resolve(target, from);
      if (!dest) continue;
      if (isNote(dest)) {
        const pub = published.has(dest);
        if (!pub) leaks.push({ kind: l.embed ? "unpublished_embed" : "unpublished_link", from, target: dest });
        if (isWikilink(l.original)) {
          // Never write an unpublished path here: null means "not public".
          (hints[from] ??= {})[target] = pub ? dest : null;
        }
        continue;
      }
      if (!allowedPath(rules, dest)) continue;
      if (!attachmentAllowed(rules, dest)) {
        leaks.push({ kind: "excluded_attachment", from, target: dest });
        continue;
      }
      include.add(dest);
    }
  }

  const files: ManifestFile[] = [];
  const content = new Map<string, () => Promise<ArrayBuffer>>();
  for (const p of include) {
    const f = all.get(p);
    if (!f) continue;
    let hash = cache.get(p, f.mtime, f.size);
    if (!hash) {
      hash = await sha256(await vault.read(p));
      cache.set(p, f.mtime, f.size, hash);
    }
    files.push({ path: p, hash, size: f.size, mtime: iso(f.mtime) });
    content.set(p, () => vault.read(p));
  }
  cache.prune(new Set(all.keys()));

  const settingsNote = findSettingsNote(all.keys());
  if (settingsNote) {
    // Pushed under a fixed name, so the server needn't know where it lives.
    const f = all.get(settingsNote)!;
    let hash = cache.get(settingsNote, f.mtime, f.size);
    if (!hash) {
      hash = await sha256(await vault.read(settingsNote));
      cache.set(settingsNote, f.mtime, f.size, hash);
    }
    files.push({ path: SITE_NOTE_PATH, hash, size: f.size, mtime: iso(f.mtime) });
    content.set(SITE_NOTE_PATH, () => vault.read(settingsNote));
  }

  const siteToml = await vault.readHidden(SITE_CONFIG_PATH);
  if (siteToml) {
    files.push({ path: SITE_CONFIG_PATH, hash: await sha256(siteToml.data), size: siteToml.data.byteLength, mtime: iso(siteToml.mtime) });
    content.set(SITE_CONFIG_PATH, async () => siteToml.data);
  }
  if (Object.keys(hints).length > 0) {
    const data = new TextEncoder().encode(stableJSON({ version: 1, notes: hints })).buffer as ArrayBuffer;
    const newest = files.reduce((t, f) => (f.mtime > t ? f.mtime : t), iso(0));
    files.push({ path: HINTS_PATH, hash: await sha256(data), size: data.byteLength, mtime: newest });
    content.set(HINTS_PATH, async () => data);
  }

  files.sort((a, b) => (a.path < b.path ? -1 : a.path > b.path ? 1 : 0));
  return { files, decisions, leaks, content, settingsNote };
}

/** JSON with sorted object keys, so equal hints hash equally. */
export function stableJSON(v: unknown): string {
  if (v === null || typeof v !== "object") return JSON.stringify(v);
  if (Array.isArray(v)) return "[" + v.map(stableJSON).join(",") + "]";
  const o = v as Record<string, unknown>;
  return "{" + Object.keys(o).sort().map((k) => JSON.stringify(k) + ":" + stableJSON(o[k])).join(",") + "}";
}
