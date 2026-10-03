// SHA-256 with WebCrypto (available on desktop and mobile Obsidian).

export async function sha256(data: ArrayBuffer): Promise<string> {
  const digest = await crypto.subtle.digest("SHA-256", data);
  const bytes = new Uint8Array(digest);
  let hex = "";
  for (const b of bytes) hex += b.toString(16).padStart(2, "0");
  return "sha256:" + hex;
}

/** HashCache remembers hashes by (path, mtime, size) to avoid rehashing. */
export interface HashEntry {
  mtime: number;
  size: number;
  hash: string;
}

export class HashCache {
  private entries: Map<string, HashEntry>;
  constructor(initial?: Record<string, HashEntry>) {
    this.entries = new Map(Object.entries(initial ?? {}));
  }
  get(path: string, mtime: number, size: number): string | undefined {
    const e = this.entries.get(path);
    return e && e.mtime === mtime && e.size === size ? e.hash : undefined;
  }
  set(path: string, mtime: number, size: number, hash: string): void {
    this.entries.set(path, { mtime, size, hash });
  }
  /** Drop entries for paths that no longer exist. */
  prune(keep: Set<string>): void {
    for (const p of this.entries.keys()) if (!keep.has(p)) this.entries.delete(p);
  }
  toJSON(): Record<string, HashEntry> {
    return Object.fromEntries(this.entries);
  }
}
