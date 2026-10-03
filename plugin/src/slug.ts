// URL of a published note, mirroring internal/slug and model.noteURL in the
// Go server: lowercase, runs of non-letters/digits become "-", folder notes
// get the folder URL, frontmatter permalink wins.

export function slug(s: string): string {
  let out = "";
  let dash = false;
  for (const ch of s.normalize("NFC")) {
    if (/[\p{L}\p{N}]/u.test(ch)) {
      if (dash && out) out += "-";
      dash = false;
      out += ch.toLowerCase();
    } else if (/\p{M}/u.test(ch)) {
      out += ch;
    } else {
      dash = true;
    }
  }
  return out;
}

export function slugPath(p: string): string {
  return p.replace(/\.md$/i, "").split("/").map(slug).filter(Boolean).join("/");
}

export function noteURL(path: string, permalink?: unknown): string {
  if (typeof permalink === "string" && permalink.trim() && !permalink.includes("..")) {
    const u = slugPath(permalink.replace(/^\/+|\/+$/g, ""));
    return u ? `/${u}/` : "/";
  }
  const i = path.lastIndexOf("/");
  const dir = i >= 0 ? path.slice(0, i) : "";
  const stem = path.slice(i + 1).replace(/\.[^.]*$/, "");
  const parent = dir.slice(dir.lastIndexOf("/") + 1);
  if (stem.toLowerCase() === "index" || (dir && stem === parent)) {
    return dir ? `/${slugPath(dir)}/` : "/";
  }
  return `/${slugPath(path)}/`;
}
