# AGENTS.md

Guide for AI coding agents (and humans in a hurry) working on kvist. Read
this first; it points to the longer docs when you need them.

## What kvist is

kvist publishes selected notes of an Obsidian vault as a static website
("digital garden") on a self-hosted server. Two programs:

- **Server** (`cmd/kvist`, `internal/`, `themes/`): one Go binary. It takes
  pushes over a small HTTPS protocol, stores them as content-addressed
  revisions, re-checks what may be public, builds the site with a theme and
  serves it.
- **Obsidian plugin** (`plugin/`): TypeScript, Obsidian API only. It decides
  what is public (gate 1), uploads it and shows a report of what stays
  private.

The hard rule of the whole project: **nothing private may ever reach the
built site.** Most of the structure below exists to guarantee that.

## Commands

```sh
make check          # everything CI runs: gofmt check, vet, Go tests with -race, plugin typecheck/tests/build
make test           # go vet + go test -race ./...
make fmt            # gofmt -w (CI fails on unformatted Go)
make plugin-test    # npm ci, tsc --noEmit, plugin tests (the e2e test builds and runs the Go server)
make build          # bin/kvist
make dev            # preview example-vault at http://127.0.0.1:1313 with live reload

go test ./internal/model -run TestNoLeaks         # one package / one test
go run ./cmd/kvist build --dir example-vault --config kvist.example.toml --emit-model - | less   # inspect the content model
go run ./cmd/kvist push --dir VAULT --server URL --site ID   # Go reference client (token in KVIST_TOKEN)
```

Requirements: Go 1.26+ (CI uses 1.27), Node 20+ (CI uses 22). The full Go
suite takes well under a minute; the plugin suite a few seconds after
`npm ci`. Always finish with `make check` (or at least `make fmt test` for
Go-only changes and `make plugin-test` for plugin changes).

## Map of the code

Pipeline: plugin/`kvist push` → `api` → `syncer` → `store` → `source/push`
→ `build` (`vault` parse → `publish` filter → `resolve` links → `model` →
`render` with a theme) → `builds/<id>/` swapped atomically into `public/`
→ `serve`.

| Path | What lives there |
|---|---|
| `cmd/kvist/` | CLI: `serve push build dev token site rollback gc version` (one file per command) |
| `internal/protocol/` | Sync protocol v1 wire types and constants (mirrored by `plugin/src/protocol.ts`) |
| `internal/api/` | HTTP handlers under `/api/v1/`, auth middleware, errors; `e2e_test.go` drives the whole server |
| `internal/syncer/` | Server side of a sync: manifest, missing blobs, commit, base/regression checks |
| `internal/store/` | On-disk store: CAS blobs, revisions, sessions, locks, GC |
| `internal/auth/` | Per-site push tokens (hashed) |
| `internal/source/` | `Source`/`Snapshot` interfaces; `push` (the store) and `dir` (a folder, for `dev`/`build`/tests) |
| `internal/vault/` | Parse notes: frontmatter, tags, links, canvases, image properties; masking of comments/code |
| `internal/markdown/` | goldmark extensions: wikilinks, embeds, callouts, tags, highlights, math, video, code block extras |
| `internal/publish/` | Publish rules, gate 2 (mirrored by `plugin/src/rules.ts`) |
| `internal/resolve/` | Obsidian link resolution |
| `internal/slug/` | URL slugs and heading anchors (mirrored by `plugin/src/slug.ts`) |
| `internal/model/` | Content model v1: the only thing themes see. Builder, backlinks, graph, search, feeds, `_site.md` settings, analytics, canvases |
| `internal/render/` | Theme loading, template funcs, page rendering, generated files (RSS, sitemap, search index), social images, i18n |
| `internal/build/` | Build orchestration, per-site queue, redirects for moved notes, atomic output |
| `internal/devserver/` | `kvist dev`: build a folder, serve it, poll for changes, live reload |
| `internal/serve/` | Static serving of each site's `public/` |
| `internal/imagemeta/` | Strip EXIF/GPS from images |
| `internal/cdn/` | Cloudflare cache purge after builds |
| `internal/pushclient/` | Go reference client: scan a folder like the plugin does, push it |
| `internal/config/` | Server TOML config: load and validate |
| `themes/garden/` | The built-in theme (embedded via `themes/embed.go`): `templates/`, `static/`, `i18n/*.toml`, `theme.toml` |
| `plugin/src/` | `main.ts` (Obsidian glue, settings note template), `publisher.ts` (flow), `scan.ts` (manifest), `rules.ts`, `slug.ts`, `canvas.ts`, `client.ts`, `i18n/` |
| `plugin/test/` | Node tests bundled by `run.mjs`; `memvault.ts` fakes a vault; `e2e.test.ts` runs against a real server |
| `testdata/leaks/basic/` | Leak fixtures: every file with `LEAKMARK` in it must never appear in output |
| `testdata/parity/` | JSON fixtures both Go and plugin tests run (rules, URLs, image properties) |
| `example-vault/` | Demo vault showing every feature, with deliberate private notes |
| `docs/*.html` | User guide (GitHub Pages, hand-written HTML, no generator) |
| `docs/*.md` | Developer notes: `design.md`, `protocol.md`, `content-model.md`, `themes.md`, `deploy.md` |
| `deploy/` | Docker Compose (plain and with Caddy), Caddyfile, systemd unit, sample `kvist.toml` |

## Invariants (break one and the change is wrong)

1. **No leaks.** Anything that reads notes (a page type, generated file,
   template func, model field, warning text) works from the published
   model only. New vector → add a fixture to `testdata/leaks/basic` whose
   content contains `LEAKMARK`, and keep the leak tests in `internal/model`
   and `internal/build` passing. Links to private notes become plain text;
   private titles and paths never appear.
2. **Two gates, one decision.** The plugin (gate 1) and the server (gate 2)
   implement the rules independently and must agree. Changing rules, slugs,
   canvas links or image properties means changing the Go side and the TS
   side together and extending `testdata/parity/*.json`. Mirror pairs:
   - `internal/publish` ↔ `plugin/src/rules.ts`
   - `internal/slug` + `model.noteURL` ↔ `plugin/src/slug.ts`
   - `internal/vault/canvas.go` ↔ `plugin/src/canvas.ts`
   - `internal/vault/image.go` ↔ `imageProperty` in `plugin/src/rules.ts`
   - `internal/pushclient/scan.go` ↔ `plugin/src/scan.ts`
   - `internal/protocol` ↔ `plugin/src/protocol.ts`
3. **Contracts are versioned.** The sync protocol (`docs/protocol.md`) and
   the content model (`docs/content-model.md`) accept additive changes;
   anything else needs a version bump and a doc entry.
4. **Deterministic builds.** Same input → byte-identical output. No wall
   clock, map iteration order or randomness in output; sort everything.
5. **Atomic.** A site is the old build or the new one, never a mix.
6. **Few dependencies.** Server: goldmark, chroma, yaml.v3, BurntSushi/toml,
   x/text, x/image, stdlib `net/http`. Plugin: the Obsidian API only (no
   runtime npm deps). Ask before adding one.

## Recipes: what else to touch

| Change | Also update |
|---|---|
| Server config key | `internal/config` (+ validation, test), `kvist.example.toml`, `deploy/kvist.toml` if users need it, `docs/configuration.html` |
| `_site.md` setting | `internal/model/settings.go`, the template `SETTINGS_PROPERTIES` in `plugin/src/main.ts`, `docs/customizing.html`, `example-vault/Kvist/_site.md` |
| Theme param | `themes/garden/theme.toml` `[params]` (with a comment), templates, `docs/themes.md` / `docs/themes.html` |
| Word in the theme | `themes/garden/i18n/en.toml` first, then every other language (tests fail until all have it) |
| Word in the plugin | `plugin/src/i18n/en.ts` first, then `sv.ts` (`plugin/test/i18n.test.ts` checks) |
| Markdown/Obsidian syntax | `internal/markdown` (+ test), a note in `example-vault/Kvist/Features/`, `docs/writing.html` |
| Content model field | `internal/model/types.go`, `docs/content-model.md`, leak check that it can't carry private data |
| Protocol field/endpoint | `internal/protocol`, `internal/api`/`syncer`, `plugin/src/protocol.ts` + `client.ts`, `internal/pushclient`, `docs/protocol.md` |
| CLI command or flag | `cmd/kvist/*.go` (+ the `usage` text in `main.go`), `docs/server.html` |
| Feature done or added | `todo.md` and `docs/roadmap.html` together; README feature list if user-visible |
| New user-guide page | Copy an existing `docs/*.html`; the sidebar nav is repeated in every page, so add the link to all of them |
| Plugin release | Bump `version` in `plugin/manifest.json` and `plugin/package.json`, then tag `vX.Y.Z` (see `plugin/README.md`) |

## Conventions

- **Go:** stdlib style, `gofmt`, small packages under `internal/`. Every
  package has a `// Package x …` doc comment; comments explain why and
  reference design sections like `(§5.2)`. Table tests, fixtures under
  `testdata/`. Errors and logs are in English.
- **TypeScript:** strict `tsc`, no runtime deps, plugin code that can be
  tested is kept Obsidian-independent (`scan.ts`, `rules.ts`, …) and
  `main.ts` adapts the real vault.
- **Writing (docs, UI text, comments):** plain, short sentences, second
  person, no marketing words. User-facing words go through i18n.
- **Commits:** small and focused. Subject like `Area: what changed, in
  plain words` (e.g. `Folder notes: keep them out of note lists`), body
  says why. Run `make check` before pushing; CI runs the same checks and
  publishes the Docker image from `main`.

## Where to read more

- `docs/design.md`: architecture, threat model (§5.3), build pipeline, decisions.
- `docs/protocol.md`: sync protocol v1 reference.
- `docs/content-model.md`: what themes receive.
- `docs/themes.md`: writing and overriding themes.
- `docs/deploy.md`: running the server.
- `CONTRIBUTING.md`: the rules for changes, in short.
- `todo.md`: open work, compared against Quartz and Obsidian Publish.
