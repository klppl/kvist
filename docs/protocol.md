# kvist sync protocol, version 1

This is the contract between a client (the Obsidian plugin, `kvist push`)
and a kvist server. The rationale is in [design.md §3](design.md#3-sync-protocol-v1);
this document is the reference. The Go wire types live in
`internal/protocol`, and `internal/pushclient` is a complete client.

## Conventions

- Base path `/api/v1/`. JSON bodies are UTF-8, `Content-Type: application/json`.
- Every request sends `Kvist-Protocol: 1`. A missing or unsupported value is
  rejected with `400 protocol_unsupported`, except on `GET /api/v1/info`.
- Every response carries `Kvist-Protocol` and `Kvist-Server-Version`.
- Authenticated endpoints need `Authorization: Bearer <token>`. Tokens look
  like `kvist_<43 chars>` and belong to one site.
- Unknown JSON fields must be ignored by both sides.
- Hashes are `sha256:` followed by 64 lowercase hex digits, over the raw bytes.
- Times are RFC 3339 in UTC.
- Clients must use HTTPS, except to `localhost`.

## Errors

Every non-2xx response has this body:

```json
{"error": {"code": "stale_state", "message": "1 note would revert to an older version; confirm to commit anyway", "details": [...]}}
```

| HTTP | `code` | Meaning / client action |
|---|---|---|
| 400 | `protocol_unsupported` | Version mismatch. Tell the user to update the plugin or the server. |
| 400 | `bad_request` | Malformed request. |
| 400 | `invalid_manifest` | `details`: `[{"path", "message"}]`. Fix the listed entries. |
| 400 | `hash_mismatch` | The uploaded bytes don't match the hash or the manifest size. Re-read the file and restart the sync. |
| 400 | `unexpected_blob` | Uploaded a hash the manifest doesn't list. |
| 401 | `unauthorized` | Bad token, token for another site, or unknown site (deliberately indistinguishable). |
| 404 | `not_found` | Unknown build id or route. |
| 409 | `rules_changed` | Fetch site info again, rebuild the manifest, start a new sync. |
| 409 | `revision_changed` | Another client committed since this sync started. Start a new sync (the blobs you uploaded are reused). |
| 409 | `missing_blobs` | `details`: hashes still missing. Upload them and commit again. |
| 409 | `stale_state` | `details`: `[Regression]`. Ask the user; to proceed, commit again with `{"force": true}`. Never force automatically. |
| 410 | `sync_expired` | The session is unknown or older than the server's `sync_ttl` (default 1 h). Start a new sync. |
| 413 | `too_large` | Body or manifest over the limits. |
| 500 | `internal` | Server error; no details are given. |

## Endpoints

### `GET /api/v1/info` (no auth)

```json
{"protocol": {"min": 1, "max": 1}, "server": "0.1.0"}
```

Clients must stop if their version is outside `[min, max]`.

### `GET /api/v1/sites/{site}`

```json
{
  "site": "garden",
  "base_url": "https://garden.example.com",
  "rules": {
    "always_public_folders": ["Kvist"],
    "exclude_folders": ["Templates", "Private"],
    "public_tag": "public",
    "private_tag": "private",
    "frontmatter_key": "publish",
    "attachment_extensions": ["png", "jpg", "pdf"]
  },
  "rules_hash": "sha256:…",
  "limits": {"max_file_size": 52428800, "max_files": 20000, "max_path_bytes": 512},
  "revision": "r000041",
  "root_folder": "Kvist"
}
```

`revision` is `""` before the first commit. `root_folder` is the vault
folder that addresses start from (`Kvist/Soil.md` is at `/soil/`), absent
when it is the vault itself; clients use it to open a note's published
page. The client applies `rules` to
choose what to push ([§ Publish rules](#publish-rules)) and echoes
`rules_hash` in the manifest.

### `POST /api/v1/sites/{site}/syncs`

Request (the manifest is the **complete** desired state):

```json
{
  "base_revision": "r000041",
  "rules_hash": "sha256:…",
  "client": {"id": "a1b2…", "name": "Alex's iPhone", "platform": "ios", "version": "0.1.0"},
  "files": [
    {"path": "Kvist/Welcome.md", "hash": "sha256:…", "size": 812, "mtime": "2026-10-02T19:12:00Z"}
  ]
}
```

- `base_revision`: the revision this client last committed (or confirmed as
  unchanged), `""` if never. Used only for the stale-state check.
- `client.id`: a random id generated once per device and kept locally
  (≤ 128 bytes). Required.
- `files`: see [§ Manifest rules](#manifest-rules).

Response:

```json
{
  "sync_id": "6f1c…",
  "missing": ["sha256:…"],
  "expires_at": "2026-10-03T11:15:12Z",
  "base_check": {"head": "r000042", "stale": true, "regressions": []}
}
```

`missing` lists hashes, not paths, sorted and without duplicates: upload
each once even if several paths share it. `base_check` is a preview of the
check that runs again at commit; a client may show it early.

### `PUT /api/v1/sites/{site}/syncs/{sync_id}/blobs/{hash}`

Body: the raw bytes. `204` on success. The server streams to a temporary
file and stores it only if the size and hash match. Uploading a hash that is
already stored succeeds without reading the body. Uploads may run in
parallel and may be retried freely.

### `POST /api/v1/sites/{site}/syncs/{sync_id}/commit`

Request body (optional): `{"force": true}` after the user confirmed a
`stale_state`.

The server, holding the site lock:

1. checks that HEAD is still the revision it was when the sync started
   (else `revision_changed`) and that the rules are unchanged;
2. checks that every blob is present (else `missing_blobs`);
3. runs the stale-state check (below), unless forced;
4. runs gate 2: re-evaluates the publish rules on every note and drops
   notes that fail, with a `gate_disagreement` warning each;
5. if the result equals HEAD (same paths and hashes), answers
   `"unchanged": true` without creating a revision or a build;
6. otherwise writes revision N+1, moves HEAD and queues a build.

```json
{"revision": "r000043", "build_id": "20261003T101512Z-r000043", "warnings": [
  {"code": "gate_disagreement", "path": "Inbox/x.md", "message": "pushed but not public under the server's rules (no_rule); not published"},
  {"code": "stale_base", "message": "another client pushed since this device's last sync; no older content would be restored"}
]}
```

Unchanged: `{"revision": "r000042", "unchanged": true, "warnings": []}`.

The client stores `revision` as its new `base_revision`.

### `GET /api/v1/sites/{site}/builds/{build_id}?wait=30s`

Returns the build status, waiting up to `wait` (max 60 s) for it to finish.

```json
{
  "id": "20261003T101512Z-r000043", "revision": "r000043",
  "state": "succeeded", "warnings": [],
  "queued_at": "…", "started_at": "…", "finished_at": "…"
}
```

`state` is `queued`, `running`, `succeeded`, `failed` (with `error`) or
`superseded` (with `superseded_by`: a newer commit replaced this build
before it started; follow that id). Builds of a site run one at a time.

## Manifest rules

A manifest is rejected with `invalid_manifest` if any entry breaks these:

- Path: relative, `/`-separated, Unicode **NFC**, valid UTF-8, no empty, `.`
  or `..` segments, no control characters, at most `max_path_bytes` bytes.
- No segment may start with `.` (so no `.obsidian/`, `.trash/`), except the
  reserved files `.kvist/links.json`, `.kvist/site.toml` and `.kvist/site.md`.
  `.kvist/site.md` carries the settings note (`_site.md`, wherever it lives
  in the vault); a vault may have at most one. Notes named `_site.md` are
  never published as pages (rule 0 of the publish rules).
- Extension: `.md` (any case), or one of `attachment_extensions`.
- No duplicate paths, and no two paths that differ only by case.
- `size` between 0 and `max_file_size`; the same hash always with the same size.
- At most `max_files` entries.

## Stale-state check

Runs only when `base_revision` ≠ HEAD, comparing the manifest with HEAD.
`.kvist/links.json` is ignored. A **regression** is:

- `older`: a path whose content differs and whose incoming `mtime` is older
  than the stored one;
- `removed`: a path in HEAD but not in the manifest whose stored `mtime` is
  newer than this client's last successful commit (any time, for a client the
  server hasn't seen).

```json
{"path": "Kvist/x.md", "kind": "older", "stored_mtime": "…", "incoming_mtime": "…"}
```

No regressions → the commit goes through with a `stale_base` warning.
Regressions → `409 stale_state` until the client commits with `force`.

## Publish rules

The client must apply the rules from site info exactly as the server does
(`internal/publish`). Per note, first match wins:

1. Excluded if the path is inside an `exclude_folders` entry (case-insensitive).
2. Excluded if the frontmatter can't be parsed (fail closed), or
   `<frontmatter_key>` is `false` (boolean, or the string `"false"`; the key
   matches case-insensitively and `false` wins over `true`), or the note has
   the `private_tag`.
3. Included if inside an `always_public_folders` entry (case-sensitive; `"/"`
   is the whole vault), or the note has the `public_tag`, or
   `<frontmatter_key>` is `true`.
4. Excluded otherwise.

Tags: from frontmatter `tags`/`tag` (list, or string split on commas and
spaces) and from the body as `#tag` at the start of a line or after
whitespace. Tag characters are letters, digits, marks, `_`, `-` and `/`, with
at least one non-digit. Matching is case-insensitive and exact: `#public/x`
is not `#public`. Tags in code (fenced, indented, inline) don't count. Tags
inside `%% %%` and `<!-- -->` comments **do** count, matching Obsidian, so a
hidden `%% #private %%` keeps a note private.

Attachments have no rules of their own. Push an attachment when a published
note links to or embeds it outside code and comments, unless it is inside an
`exclude_folders` folder: those are never pushed (the server drops them at
commit with a `gate_disagreement` warning).

## Hints file (`.kvist/links.json`)

Optional. Tells the server how the client resolved wikilinks:

```json
{"version": 1, "notes": {"Kvist/Welcome.md": {"Note": "Kvist/Note.md", "Diary": null}}}
```

Keys are link targets as written (without `#heading` and `|alias`). The value
is the vault path the link resolves to if that note is published, else
`null`. **Never put an unpublished path in this file.** The server only uses
a hint whose target is published.
