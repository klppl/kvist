// An in-memory VaultLike for tests, with a tiny metadata extractor good
// enough for the fixtures (Obsidian's metadataCache does this in the app).
import { LinkInfo, NoteMeta, VaultFile, VaultLike } from "../src/scan";

export class MemVault implements VaultLike {
  files_ = new Map<string, { data: Uint8Array; mtime: number }>();
  hidden = new Map<string, { data: Uint8Array; mtime: number }>();
  reads = 0;

  write(path: string, text: string | Uint8Array, mtime = Date.now()) {
    const data = typeof text === "string" ? new TextEncoder().encode(text) : text;
    (path.startsWith(".") ? this.hidden : this.files_).set(path, { data, mtime });
  }
  remove(path: string) {
    this.files_.delete(path);
  }
  rename(from: string, to: string) {
    const f = this.files_.get(from)!;
    this.files_.delete(from);
    this.files_.set(to, f);
  }
  files(): VaultFile[] {
    return [...this.files_].map(([path, f]) => ({ path, mtime: f.mtime, size: f.data.byteLength }));
  }
  async read(path: string): Promise<ArrayBuffer> {
    this.reads++;
    const f = this.files_.get(path);
    if (!f) throw new Error("missing " + path);
    return f.data.slice().buffer as ArrayBuffer;
  }
  meta(path: string): NoteMeta | null {
    const f = this.files_.get(path);
    if (!f || !path.endsWith(".md")) return null;
    let text = new TextDecoder().decode(f.data);
    let frontmatter: Record<string, unknown> | undefined;
    const fm = /^---\n([\s\S]*?)\n---\n?/.exec(text);
    if (fm) {
      frontmatter = {};
      for (const line of fm[1].split("\n")) {
        const m = /^([\w-]+):\s*(.*)$/.exec(line);
        if (m) frontmatter[m[1]] = m[2] === "true" ? true : m[2] === "false" ? false : m[2];
      }
      text = text.slice(fm[0].length);
    }
    const body = text.replace(/`[^`]*`/g, "");
    const tags = [...body.matchAll(/(?:^|\s)#([\p{L}\p{N}_\-/]+)/gu)].map((m) => "#" + m[1]);
    const links: LinkInfo[] = [...body.matchAll(/(!?)\[\[([^\]|]+)(?:\|[^\]]*)?\]\]/g)].map((m) => ({
      link: m[2], original: m[0], embed: m[1] === "!",
    }));
    return { frontmatter, tags, links };
  }
  resolve(linkpath: string, source: string): string | null {
    const want = linkpath.toLowerCase();
    const names = [...this.files_.keys()];
    return (
      names.find((p) => p.toLowerCase() === want || p.toLowerCase() === want + ".md") ??
      names.find((p) => {
        const base = p.slice(p.lastIndexOf("/") + 1).toLowerCase();
        return base === want || base === want + ".md";
      }) ??
      null
    );
  }
}
