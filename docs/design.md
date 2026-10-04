# kvist — design document

Status: **approved; Phases 1–7 implemented** (GitHub Pages publishing not done; see §11) · Protocol v1 ([reference](protocol.md)) · Content model v1 · 2026-10-03
· Go module `github.com/klppl/kvist`

kvist publishes selected notes from an Obsidian vault as a themeable static
website (a "digital garden"). A companion Obsidian plugin decides what is
public and pushes it; a self-hosted Go service stores it, re-checks it, builds
the site and serves it.

---

## 1. Goals and non-goals

**Goals (v1)**

- Publish from inside Obsidian (desktop and mobile) without other tools.
- Never leak a private note — through links, embeds, attachments, backlinks,
  graph, search, tags, feeds, sitemap, metadata or errors.
- Deterministic builds: the same pushed content + config + theme always yields
  byte-identical output, whichever client pushed it.
- Atomic: a site is either the old version or the new version, never a mix.
- One static Go binary; runs on any Linux box, in Docker, behind Cloudflare.
- Themes are plain folders consuming a documented, versioned content model.

**Non-goals (v1)** — designed for, not built: a watched-folder content source,
Dataview snapshots, Canvas, multilingual sites, multiple sites per vault,
Cloudflare/GitHub Pages publishing (Phase 7 bonus), Hugo export, comments,
server-side math rendering, an admin web UI.

---

## 2. Architecture

```
 ┌──────────── Obsidian (any device) ────────────┐
 │  metadataCache ─► publish rules ─► manifest   │   gate 1: decide
 │  leak report · status bar · publish toggle    │
 └───────────────────────┬───────────────────────┘
                         │ HTTPS, per-site token, protocol v1
                         ▼
 ┌──────────────────── kvist (Go) ─────────────────────────────────────┐
 │  API ─► Source "push" ─► content store (CAS blobs + revisions)      │
 │                               │ commit → revision N                 │
 │                               ▼                                     │
 │  build pipeline:                                                    │
 │   snapshot ─► parse ─► publish filter ─► resolve ─► model ─► render │   gate 2: enforce
 │                          (gate 2)                         │         │
 │                                                           ▼         │
 │                     builds/<id>/  ──atomic symlink swap──►  public/ │
 └───────────────────────────────────────────────────────────┬─────────┘
                                                             ▼
                              Caddy / nginx / built-in static server ─► Cloudflare
```

### 2.1 Responsibilities

| Component | Decides | Does not |
|---|---|---|
| Plugin | Which files are published (using Obsidian's own tags, frontmatter, resolved links); what to upload; surfaces leaks | Render HTML, know about themes |
| Server API / store | Authenticates, validates paths, stores content atomically, orders commits | Parse anything at upload time beyond hashing |
| Build pipeline | Re-applies publish rules independently, resolves links, renders | Trust the plugin's verdicts |
| Theme | Presentation | See raw vault files, private notes, or unfiltered frontmatter |

### 2.2 Content sources

The build pipeline never talks to HTTP. It reads a **snapshot** from a source:

```go
// package source
type Source interface {
    // Snapshot returns the current committed state. It must be immutable:
    // later commits produce new snapshots, never mutate old ones.
    Snapshot(ctx context.Context) (Snapshot, error)
    // Changes delivers a value whenever a new snapshot is available.
    Changes() <-chan struct{}
}

type Snapshot interface {
    Revision() string               // opaque, monotonic per source
    Files() []File                  // sorted by Path
    Open(f File) (io.ReadCloser, error)
    Hints() *Hints                  // optional client hints (see §3.6); nil if none
}

type File struct {
    Path  string // vault-relative, NFC, forward slashes
    Hash  string // "sha256:<hex>"
    Size  int64
    MTime time.Time // informational only
}
```

v1 ships `source/push` (backed by the content store) and a **dev-only,
read-only** `source/dir` used by `kvist dev --vault <dir>` and tests (no
watching for production use, not exposed in server config). A production `source/dir`
(Git checkout, Syncthing, plain folder) implements the same interface by
walking a directory, hashing files and watching with fsnotify — no pipeline
changes. Since that source has no plugin in front of it, gate 2 is its only
gate; this is why gate 2 must be complete on its own.

---

## 3. Sync protocol (v1)

Manifest reconciliation over HTTPS/JSON. All endpoints are under
`/api/v1/`. Every request carries `Kvist-Protocol: 1`; every response carries
`Kvist-Protocol` and `Kvist-Server-Version`.

### 3.1 Flow

```
plugin                                            server
  │ GET  /api/v1/info                               │  versions (no auth)
  │ GET  /api/v1/sites/{site}                       │  rules, rules_hash, limits, revision
  │ POST /api/v1/sites/{site}/syncs   {manifest}    │  → {sync_id, missing[], base_check}
  │ PUT  /api/v1/sites/{site}/syncs/{id}/blobs/{h}  │  raw bytes, verified against h
  │   … (only missing hashes, resumable, parallel)  │
  │ POST /api/v1/sites/{site}/syncs/{id}/commit     │  → {revision, build_id, warnings[]}
  │ GET  /api/v1/sites/{site}/builds/{id}?wait=30s  │  → status, warnings[] (long-poll)
```

1. **Handshake.** `GET /info` returns `{protocol: {min: 1, max: 1}, server}`.
   The plugin refuses to continue if its version is outside the range and shows
   "update the plugin/server". Unknown future fields are ignored by both sides.
2. **Site info.** Returns the server's publish rules and their hash, size
   limits, and the current revision. The **server is the single source of
   truth for publish rules**; the plugin applies the rules it received, so both
   gates run the same rules (but independent implementations).
3. **Manifest.** The plugin sends the full desired state:

   ```json
   {
     "base_revision": "r000041",
     "rules_hash": "sha256:…",
     "client": {"id": "a1b2…", "name": "Alex's iPhone", "platform": "ios", "version": "0.1.0"},
     "files": [
       {"path": "Kvist/Welcome.md", "hash": "sha256:…", "size": 812, "mtime": "2026-10-02T19:12:00Z"},
       {"path": "attachments/map.png", "hash": "sha256:…", "size": 40211, "mtime": "…"}
     ]
   }
   ```

   The server validates (see §3.4) and answers with the **hashes** it lacks.
   Missing is by hash, not path: renames and moves upload nothing.
4. **Upload.** `PUT …/blobs/sha256:<hex>` with the raw body. The server streams
   to a temp file, verifies hash and size, then renames into the blob store.
   A blob is either complete or absent. Re-uploading is idempotent.
5. **Commit.** The server re-checks that every manifest hash is present,
   checks the base (§3.3), applies gate 2 note-level rules (§5.2), then
   atomically writes revision N+1 and moves `HEAD`. It enqueues a build and
   returns immediately with a `build_id`. Nothing visible changes before this.
   If the result equals `HEAD` (same paths and hashes), no revision or build
   is created and the response says `unchanged`.
6. **Delete by omission.** Anything not in the committed manifest is not in the
   revision, so the next build doesn't contain it. Unreferenced blobs are
   garbage-collected after the retention window (§3.5).

### 3.2 Correctness cases

| Case | Why it works |
|---|---|
| Unpublish (remove `#public`) | File absent from next manifest → absent from revision → absent from build. |
| Rename / move | Same hash, new path → zero upload; old path absent → gone. |
| Missed push | Each manifest is the complete state; the next successful sync heals any drift. |
| Interrupted upload | Blobs are invisible until commit; sessions expire (default 1 h) and their unreferenced blobs are GC'd. Retrying the sync re-uses already uploaded blobs. |
| Two devices pushing at once | Commit is compare-and-swap on the revision seen at session start; the loser gets `409 revision_changed` and retries from step 3. |
| Failed build | Revision is committed but `public/` still points at the last good build; status reports the failure. |

### 3.3 Stale-state protection ("don't silently roll back")

LiveSync means device B may hold *newer* notes than the last thing it pushed,
or *older* ones if it hasn't synced yet. A plain "base revision must match"
check would reject the good case. So the commit does two checks:

- **Base check:** `base_revision` ≠ current `HEAD` means another client pushed
  since this one last synced.
- **Regression check** (only when the base is stale): compare against `HEAD`:
  - a path whose incoming `mtime` is older than the stored one, with different content;
  - a path present in `HEAD` but missing from the manifest, whose stored
    `mtime` is newer than the client's last successful push.

Outcome: stale base **without** regressions → accepted, noted in warnings.
Stale base **with** regressions → `409 stale_state` with a summary ("3 notes
would revert to older versions, 1 would be removed"). The plugin shows a
confirmation dialog listing them; the user can retry with `"force": true`.
Auto-publish never forces; it pauses and turns the status bar amber.

`mtime` is a heuristic (clock skew, sync tools rewriting mtimes). It can only
cause an extra confirmation, never a silent loss, so that failure mode is
acceptable.

### 3.4 Validation and limits

- Paths: relative, forward slashes, Unicode NFC (macOS produces NFD), no `..`,
  no empty segments, no control characters, ≤ 512 bytes. Two paths differing
  only by case are rejected (they collide on case-insensitive filesystems and
  in URLs).
- Only allowed extensions (`.md` plus a configurable attachment list).
- Size limits per file (default 50 MiB, below Cloudflare's 100 MB proxy limit)
  and per manifest (default 20 000 files).
- `rules_hash` mismatch → `409 rules_changed`; the plugin refetches rules.

### 3.5 Storage layout (server)

```
<data>/
  tokens.json                      # hashed tokens, managed by CLI only
  sites/<site>/
    blobs/sha256/ab/cdef…          # content-addressed, write-once
    revisions/r000042.json         # committed manifests, immutable
    HEAD                           # "r000042", replaced via rename(2)
    syncs/<id>.json                # open sessions (expire)
    builds/20261003T101512Z-r000042/
    public -> builds/…             # the only directory ever served
    cache/                         # incremental build cache
    status.json                    # last build result + warnings
```

Retention: keep the last N revisions (default 10) and last M builds
(default 3), enabling `kvist rollback <site> <revision>`. Consequence:
unpublished content leaves the *website* immediately but stays on the
*server's disk* until it falls out of retention. Documented; configurable to 1.

### 3.6 Client hints (link resolution)

Obsidian resolves `[[Note]]` against the whole vault. The server only sees the
published subset, so if `Private/Note.md` and `Public/Note.md` both exist,
Obsidian may resolve to the private one while the server would pick the public
one — a wrong link. To make both agree, the plugin may include a hints file in
the manifest at the reserved path `.kvist/links.json`:

```json
{"version": 1, "notes": {"Kvist/Welcome.md": {"Note": "Public/Note.md", "Secret": null}}}
```

The plugin writes `null` for any target that is not published, so **private
paths are never sent**. The server uses a hint only if its target is in the
published set, and falls back to its own resolver otherwise or when no hint
exists (e.g. a future folder source). Hints can make a link *less* resolved,
never resolve to something unpublished.

### 3.7 Authentication

- Per-site tokens, created/revoked only via CLI on the server
  (`kvist token create --site garden --name laptop`). Shown once, stored as
  SHA-256 (tokens are 256-bit random, so a slow KDF adds nothing).
- Scope in v1: `push` (sync + build status for one site). There is no HTTP API
  for config, tokens or themes — a stolen token can change content, never the
  server. Rollback undoes damage.
- TLS: normally terminated by Caddy/Cloudflare; `kvist serve` can also use
  cert files directly. The plugin refuses `http://` except to `localhost`.
- Errors are generic for unauthenticated callers; detailed warnings (which can
  name notes) are only returned to a valid token for that site.

---

## 4. Build pipeline

```
Snapshot ─► Parse ─► Publish filter ─► Resolve ─► Model ─► Render ─► Output
```

| Stage | Package | Input → Output | Notes |
|---|---|---|---|
| Parse | `vault` | bytes → `ParsedNote` (frontmatter, tags, links, embeds, headings, block IDs, AST) | Pure, cached by content hash. Obsidian-flavored goldmark extensions live in `markdown`. |
| Publish filter | `publish` | parsed notes + rules → published set, attachment set, warnings | Gate 2. Note-local rules, then attachment reachability from published notes only. |
| Resolve | `resolve` | links/embeds → targets within the published set | Obsidian semantics: path, basename, aliases, `#heading`, `#^block`, `|alias`. Unresolved ⇒ treated identically whether missing or private. |
| Model | `model` | everything → `content model v1` (§6) | Backlinks, tags, folder tree, graph, search docs derived here, from published data only. |
| Render | `render` | model + theme → files | `html/template`, chroma highlighting, theme static assets. |
| Output | `build` | files → `builds/<id>` → swap `public` | Atomic; unchanged files hard-linked from the previous build. |

**Incremental builds (as built in Phase 5).** Each build still renders every
page: a 5 000-note vault builds in about 7 s. A full render is simple and always
correct, and the folder tree and backlinks make most pages depend on many
notes anyway. What is incremental is the I/O: files whose bytes match the
previous build, and attachments already present (their URLs are
content-hashed), are hard-linked instead of written, so large attachments
are neither re-read nor re-stripped, and retained builds share disk space.
The page-level cache below remains an option if render time becomes a
problem.

*Original plan:* Parsing is cached by blob hash. Resolution and model
building are global but cheap (no I/O, no rendering). Each output page has a
key = hash(note content, resolved dependency signature — embedded notes, link
targets' URL/title, backlinks —, theme hash, site config hash). Unchanged keys
are hard-linked from the previous build instead of re-rendered. The output is
still a fresh directory, so atomicity is kept.

**Determinism.** Sorted iteration everywhere, no wall-clock in output except
the commit time of the revision, stable slugs, stable JSON key order.

**Markdown features (v1).** CommonMark + GFM (tables, task lists,
strikethrough, autolinks), footnotes, frontmatter (YAML), wikilinks
`[[a]] [[a|b]] [[a#h]] [[a#^id]]`, embeds `![[note]] ![[note#h]]
![[img.png|300]] ![[doc.pdf]]`, markdown links to notes/attachments, inline and
frontmatter tags (nested `#a/b`), callouts (all Obsidian types, foldable
`+`/`-`, custom titles, nesting), `==highlight==`, `%%comments%%` (stripped),
block IDs `^id`, heading anchors matching Obsidian's `#Heading` links, code
highlighting (chroma, CSS classes, light+dark), math `$…$ $$…$$` and
` ```mermaid ` (see §7).

---

## 5. Publish rules and leak prevention

### 5.1 Rules

Evaluated per note; order matters:

1. **Excluded** if in an `exclude_folders` entry (e.g. `templates/`,
   `private/`) — *addition: a folder-level private override.*
2. **Excluded** if frontmatter `publish: false` or tag `#private`.
3. **Included** if in an `always_public_folders` entry, or tag `#public`, or
   frontmatter `publish: true`.
4. Otherwise excluded.

Tags match Obsidian semantics: case-insensitive, from body and frontmatter
`tags`/`tag`, not inside code. Tags inside `%% %%` and HTML comments *do*
count (Obsidian indexes them, and `%% #private %%` is a natural way to hide
the tag from the reading view), so a commented-out `#private` fails closed.
Links inside comments are ignored, since comment content is never rendered. Exact match only: `#public/x` does
**not** publish. Tag names and the frontmatter key are configurable. Control
tags (`#public`, `#private`) are hidden from tag pages by default.

Attachments (non-`.md`) have no rules of their own: an attachment is published
iff referenced (link or embed) by a published note's *rendered* content, and
never if it lies in an `exclude_folders` folder (the leak suite caught a
published note linking straight into `Private/`).

### 5.2 Two gates

- **Gate 1 (plugin)** builds the manifest from Obsidian's metadataCache.
- **Gate 2 (server, at commit)** parses each `.md` itself and applies the same
  rules. A note that fails is dropped from the revision and reported as a
  `gate_disagreement` warning (it indicates a bug or a rules mismatch). At build
  time the filter runs again (defense in depth for every source).

Cost of this design: a note wrongly pushed by gate 1 is transmitted and stored
on the server (never published, GC'd). The server can't judge content it hasn't
received; this is unavoidable and documented.

### 5.3 Threat model

Adversary: any website visitor (and crawlers, caches, search engines).
Asset: the existence, title, path, content and metadata of private notes.
Trusted: the server operator, the theme, the plugin (but verified).

| Vector | Mitigation |
|---|---|
| Link to private note | Rendered as muted text (`<span class="link-unpublished">`), identical to a link to a nonexistent note — no existence oracle. Warning (configurable: `ignore`/`warn`/`error`). Text shown is the alias if any, else the link text the author wrote in the published note. |
| Embed of private note/section | Omitted entirely (configurable neutral placeholder without title). Warning. Recursion through published embeds is checked at every level; cycle + depth limit. |
| Attachments | Only reachable-from-published attachments are output. Plugin pushes only those; server re-derives. |
| Image metadata | EXIF/XMP/IPTC/comments/text chunks (GPS, device, author) stripped from JPEG/PNG/WebP without re-encoding (configurable); JPEG orientation is kept as a minimal EXIF block. A file that can't be parsed is not published. SVG, GIF, AVIF and PDF are copied as-is: **residual risk, documented**. |
| Backlinks | Computed from published notes only. |
| Graph | Published nodes and edges only; **no ghost nodes** for unresolved links. |
| Search index | Built from rendered, filtered text of published notes. |
| Tags pages / tag cloud | From published notes only; control tags hidden. |
| RSS, sitemap, OpenGraph | Generated from the model, which only contains published notes. |
| Frontmatter | Not exposed raw. Themes get standard fields + an allowlist (`expose_frontmatter`). Unknown keys never reach HTML. |
| Comments | `%%…%%` stripped at parse; HTML comments stripped at render. |
| Hints file | Contains only published paths or `null` (§3.6); never output. |
| Error pages / API | Static generic 404; API errors don't echo content to unauthenticated callers. |
| Old builds | Only `public/` is served; config docs point web servers at it, never at the site dir. |
| CDN caches | After unpublish, Cloudflare may serve a stale page until TTL. Defaults: HTML `Cache-Control: max-age=60`; hashed assets immutable. Optional Cloudflare cache purge after build (Phase 7). **Residual risk, documented.** |
| Search engines / archives | Out of our control once published. Documented. |
| Future features | Dataview snapshots and Canvas must go through the same filter (results referencing non-published notes are dropped). Noted in the model contract. |

**Leak tests** are first-class: `testdata/leaks/` contains vaults where every
vector above is exercised; a test builds the site and asserts that no output
file (HTML, JSON, XML, JS, image metadata) contains any marker string planted
in private notes (titles, paths, body, frontmatter values).

---

## 6. Content model v1

The contract between pipeline and themes. Exposed to templates as Go structs
and serializable to JSON (`kvist build --emit-model`), which also enables a
future Hugo export and non-Go renderers. Documented in `docs/content-model.md`.
Breaking changes bump `ModelVersion`; themes declare the versions they support.

```
Site
  ModelVersion  int
  Config        SiteConfig      title, description, base_url, language, author, theme params
  Revision      string          + BuiltAt (commit time of revision → deterministic)
  Notes         []*Note         sorted by URL
  Assets        []*Asset
  Tags          []*Tag          tree for nested tags
  Root          *Folder         folder tree (published notes only)
  Graph         Graph           nodes (note IDs) + edges
  Home          *Note           configurable, else generated index

Note
  ID, Path, URL, Slug, Title    title: frontmatter `title` → first H1 → file name
  Aliases, Tags []*Tag
  Created, Updated              frontmatter dates, else mtime of revision
  Description                   frontmatter or first paragraph
  Params        map             allowlisted frontmatter only
  Content       template.HTML
  TOC           []*Heading      nested
  Links         []*Link         outgoing, resolved, published only
  Backlinks     []*Backlink     source note + context snippet
  Folder        *Folder
  Features      {Math, Mermaid, Code}  so themes load JS only where needed
  WordCount, ReadingTime

Asset   Path, URL, Size, MediaType, Hash
Tag     Name, Slug, URL, Notes, Children, Parent
Folder  Name, Path, URL, Notes, Children, Index *Note (folder note, if any)
```

URLs: `Kvist/My Note.md` → `/kvist/my-note/` (slugified, configurable;
frontmatter `permalink` overrides). Slug collisions are a build error naming
both notes (both are published, so naming them is safe). Attachments:
`/_assets/<hash8>/<file-name>` — content-hashed, so CDN caching can be
`immutable`.

Generated files besides pages: `/search-index.json`, `/graph.json`,
`/index.xml` (RSS, newest `Updated` first), `/sitemap.xml`, `/robots.txt`,
`/404.html`, tag pages `/tags/<tag>/`, folder pages.

---

## 7. Themes

```
themes/garden/
  theme.toml            name, version, license, model = [1], params with defaults
  templates/
    base.html  note.html  folder.html  tag.html  tags.html  home.html  404.html
    partials/…
  static/               css, js, fonts, vendored libs
```

- `html/template` with a small, documented function set (`url`, `asset`,
  `markdownify`, `dateFormat`, `toc`, …). No filesystem or network access from
  templates.
- Site config can override individual templates/static files via a local
  overrides folder, so users customize without forking.
- Default theme **garden**: digital-garden look ("planted/tended" dates,
  growth-stage badge from frontmatter `stage`), three panes (a nav pane with
  folders or tag groups, a list of the notes around the page, the page),
  backlinks panel, local + global graph, TOC, search modal, dark/light with
  system default + toggle, OpenGraph tags.
- Client-side JS is progressive enhancement: pages read fine without JS
  (except math/Mermaid/graph/search).
- Vendored, MIT-licensed libraries: MiniSearch (search) and KaTeX (math).
  The graph is a small canvas force layout in the theme's own script (no
  d3). *Changed during Phase 4:* Mermaid's bundle is ~5 MB, so it is not
  vendored. Pages with a diagram load it from a pinned jsDelivr URL with a
  subresource-integrity hash, and the `mermaid_url` theme param points it at
  a self-hosted copy for sites that want no third-party requests.

**Decision: math and Mermaid render client-side** in v1. Go has no native
KaTeX/Mermaid; embedding a JS engine (goja) for KaTeX is possible later
(output stays static). Trade-off: no-JS readers see TeX source.

---

## 8. Plugin

TypeScript, Obsidian API only (no Node/Electron APIs → works on mobile).
Uses `requestUrl` (avoids CORS on mobile) and `crypto.subtle` for SHA-256.

- **Settings:** server URL and site in plugin data (synced with the vault);
  token, device name, auto-publish, debounce (default 30 s), client id, last
  revision and the hash cache in Obsidian's per-device local storage, so
  LiveSync never copies a token or makes two devices share one client id.
  *(Changed in Phase 6; the plan kept the token in plugin data.)*
- **Manifest builder:** rules from server → published notes via metadataCache
  (`getAllTags`, frontmatter, folder) → attachments via resolved links/embeds →
  hints file. Hash cache keyed by (path, mtime, size) to avoid rehashing.
- **Commands:** "Publish now", "Toggle publish for current note" (writes
  frontmatter via `processFrontMatter`, also in file menu), "Open leak report",
  "Open published page".
- **Auto-publish:** debounced after modify/rename/delete/metadata change of
  any file that was or would become published; never forces through a
  stale-state warning.
- **Status bar:** synced (time) · pending · publishing · warnings (n) ·
  stale-state · error. Click opens the leak report.
- **Leak report view:** links/embeds to unpublished notes, missing
  attachments, gate disagreements and build warnings from the server — each
  with a jump-to-note action.

Risk: LiveSync may deliver a half-synced vault to a device that auto-publishes
mid-sync. The debounce plus the regression check (§3.3) cover most of it;
docs recommend enabling auto-publish on one primary device.

---

## 9. Configuration

TOML. One server config file, multiple sites from day one (so "multiple sites
per vault" later is a plugin change only).

```toml
data_dir = "/var/lib/kvist"
listen   = "127.0.0.1:8080"     # API (+ optional static serving)

[[site]]
id       = "garden"
base_url = "https://garden.example.com"
title    = "Alex's garden"
theme    = "garden"             # name in themes dir, or a path
serve    = true                 # serve public/ from this binary on host match

  [site.publish]
  always_public_folders = ["Kvist"]
  exclude_folders       = ["Templates", "Private"]
  public_tag  = "public"
  private_tag = "private"
  frontmatter_key = "publish"
  unpublished_links  = "warn"   # ignore | warn | error
  unpublished_embeds = "warn"
  attachment_extensions = ["png","jpg","jpeg","gif","webp","svg","pdf","mp3","mp4"]
  expose_frontmatter = ["stage", "cover"]
  strip_image_metadata = true

  [site.theme_params]
  accent = "#3f7d4e"
```

**Vault-side overrides.** An optional settings note (`_site.md`, pushed as
`.kvist/site.md`; earlier `.kvist/site.toml`) may override *presentation*
fields only: `title`, `description`, `author`, `language`, `home`, the
menu and theme parameters. Everything security-relevant
(`publish.*`, `theme`, `base_url`, paths, limits) is server-only; unknown or
forbidden keys are a build warning and are ignored. This keeps the token's
scope at "content" while letting you edit the site from Obsidian.

---

## 10. Repository layout

```
kvist/
  cmd/kvist/              CLI: serve, build, dev, push, token, rollback, version
  internal/
    config/               load + validate TOML
    protocol/             wire types, version constants (shared with test client)
    api/                  HTTP handlers, auth middleware, errors
    store/                CAS blobs, revisions, sessions, GC
    source/               Source interface; source/push
    markdown/             goldmark extensions (wikilink, embed, callout, tag, comment, highlight, blockid, math)
    vault/                parse notes → ParsedNote (pure)
    publish/              rules + reachability (gate 2)
    resolve/              Obsidian link resolution
    model/                content model types + builder (+ derive: backlinks, graph, search, feeds)
    render/               theme loading, template funcs, page rendering
    build/                orchestration, incremental cache, atomic output
    devserver/            live reload
    pushclient/           reference protocol client (used by `kvist push` and tests)
  themes/garden/
  plugin/                 Obsidian plugin (TypeScript, own package.json)
  example-vault/          demo vault: every feature + deliberate private notes
  testdata/leaks/         leak-prevention fixture vaults
  docs/                   design.md, protocol.md, content-model.md, themes.md, deploy.md
  Dockerfile  Makefile  CONTRIBUTING.md  LICENSE (MIT)
```

`internal/` keeps the Go API surface small; the public contracts are the
protocol and the content model, specified in docs and JSON. Dependencies kept
minimal: goldmark (+ highlighting/chroma), yaml.v3, BurntSushi/toml, fsnotify.
HTTP via stdlib (`net/http` routing).

The **`kvist push --dir <vault>`** reference client (Phase 1) implements the
plugin's role in Go (rules, manifest, upload, commit). It is the protocol test
harness, lets contributors work without Obsidian, and keeps the plugin honest.

---

## 11. Phases

1. Scaffold, config, store, protocol + sync API, token CLI, `kvist push` client. Tests: every §3.2 case.
2. `markdown` + `vault` + `publish` + `resolve` + `model`. Leak test suite (model-level).
3. Renderer, theme system, garden theme, atomic builds, built-in static serving. Leak suite on HTML output.
4. Search, graph, backlinks panel, RSS, sitemap, OpenGraph, attachments + metadata stripping.
5. Dev server (live reload) + incremental builds.
6. Obsidian plugin (desktop + mobile).
7. Docker, Hetzner + Cloudflare guide, optional CF/GitHub Pages publish + CF purge, CONTRIBUTING.
   *Done:* Docker, compose + Caddy, systemd unit, deploy guide, Cloudflare
   purge after each build, CI, CONTRIBUTING. *Not done:* publishing to
   Cloudflare Pages or GitHub Pages (project pages need sub-path base URLs,
   which v1 rejects).

---

## 12. Decisions challenged / trade-offs

- **Go-native vs Hugo:** agreed. Hugo can't express "resolve against a filtered
  set and refuse leaks" without fighting it; the JSON model keeps a Hugo export
  open.
- **Server is authoritative for publish rules**, plugin fetches them. Avoids
  the two gates silently running different rules.
- **Missing-by-hash, not by path** — renames are free, uploads are idempotent.
- **Async build after commit** with long-poll status, instead of blocking the
  commit on the build. Builds are serialized per site and coalesced.
- **Client-side math/Mermaid** (see §7).
- **Hashed asset URLs** trade stable attachment URLs for safe immutable CDN caching.
- **Symlink swap** requires a POSIX filesystem (fine for Linux/macOS; Windows
  servers unsupported in v1).
