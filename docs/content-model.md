# Content model v1

The content model is what a theme sees. The build pipeline creates it from
the **published notes only**, after the publish filter. A theme cannot leak a
private note because it never receives one. The Go types are in
`internal/model`. `kvist build --dir VAULT --emit-model model.json` writes the
same data as JSON, which lets you inspect it and is the basis for exports
to other generators.

Templates get the Go values. In JSON, pointers between objects (note ↔
folder, note ↔ tag, backlinks) are written as note ids.

## Site

| Field | Type | |
|---|---|---|
| `ModelVersion` | int | `1`. Themes list the versions they support in `theme.toml`. |
| `Config` | SiteConfig | Presentation settings (below). |
| `Revision` | string | The content revision that was built. |
| `BuiltAt` | time | Commit time of the revision, never the wall clock, so builds are reproducible. |
| `Notes` | []*Note | Sorted by URL. |
| `Assets` | []*Asset | Attachments referenced by rendered content or by an image property (below), sorted by URL. |
| `Tags` | []*Tag | Top-level tags; nested tags are `Children`. |
| `AllTags` | []*Tag | Every tag, sorted by name (templates only). |
| `Root` | *Folder | The site's root folder and the tree below it (published notes only). |
| `Graph` | Graph | Nodes and edges between published notes. |
| `Home` | *Note | `home` from the settings note, else the root folder's own note (`index.md`, or named like the folder); nil means the theme generates an index. |

## SiteConfig

`Title`, `Description`, `BaseURL` (no trailing slash), `Language`, `Author`,
`StrictLineBreaks` (single line breaks were rendered as spaces, not `<br>`),
`Nav` (list of `{Title, URL}`), `Params` (theme parameters), `Image` (the
default social preview image, as `Note.Image`), `Favicon` (the browser tab
icon, as `Image`; empty means the theme's own), `Analytics` (nil, or
`{Provider, ID, Script, Endpoint}`: `plausible`, `umami` or `goatcounter`,
read from the `analytics`, `analytics_id` and `analytics_url` theme
parameters and checked; `Script` is always https) and `Profile` (nil unless
set: `{Avatar, Bio, Links}`, each link `{Title, URL, Kind}` with `Kind` one of
`github`, `gitlab`, `mastodon`, `bluesky`, `linkedin`, `x`, `youtube`,
`email`, `website`).

The server config sets these. `image`, `favicon`, `avatar` (or `logo`), `bio` and
`profile_links` come from the settings note only. The vault may override `title`,
`description`, `author`, `language`, `strict_line_breaks`, `home`, the menu (`Nav`) and theme
parameters with a **settings note**: a note named `_site.md` in any folder,
never published as a page. Clients push its content as `.kvist/site.md`.

```markdown
---
title: Alex's garden
home: "[[Welcome]]"
groups: [articles, projects]
accent: "#3f7d4e"
---
- [About](/about/)
- [[Reading list]]
```

Properties other than the site fields become theme parameters; empty ones
are ignored. Obsidian's own properties (`tags`, `aliases`, `cssclasses`,
`publish`) are skipped, and server-only keys (`base_url`, publish rules, …)
are ignored with a warning. `Nav` comes from the list items in the body that
are a single link: URLs are kept, `[[wikilinks]]` and links to `.md` files resolve to published notes
(links to anything else are dropped with a warning). Links in comments and
code blocks don't count.

## Note

| Field | Type | |
|---|---|---|
| `ID` | string | URL without slashes (`garden/my-note`); `index` for `/`. |
| `Path` | string | Vault path (`Kvist/My Note.md`). |
| `URL` | string | `/my-note/` (for `Kvist/My Note.md`, with `Kvist` as the root folder). |
| `Slug` | string | URL without surrounding slashes. |
| `Title` | string | Frontmatter `title`, else the first `# Heading`, else the file name. |
| `Aliases` | []string | Frontmatter `aliases`. |
| `Tags` | []*Tag | Frontmatter tags plus tags in the body, without the control tags and without tags inside comments. |
| `Created`, `Updated` | time | `Updated`: frontmatter `updated`/`modified`/`lastmod`, else the file's modification time. `Created`: `created`/`date`, else `Updated`. |
| `Description` | string | Frontmatter `description`/`summary`, else the first paragraph (≤ 200 characters). |
| `Params` | map | Only the frontmatter keys listed in `expose_frontmatter`. Other keys never reach a theme. |
| `Image` | string | Social preview image from the `image` or `cover` property: the URL of a published image (`/_assets/…`), or an http(s) URL; empty if unset or not a published image. The value may be `"[[cover.png]]"`, a vault path, or a URL. |
| `Unlisted` | bool | Frontmatter `unlisted: true`. The note has its page, but no folder, tag, graph, backlink, search index, feed or sitemap lists it; a theme should add `<meta name="robots" content="noindex">`. Its `Tags` are only the ones listed notes carry too. |
| `Canvas` | bool | The page is an Obsidian canvas (a `.canvas` file), not a Markdown note; see [Canvases](#canvases). |
| `Content` | HTML | The rendered body. |
| `TOC` | []*Heading | Nested `{Level, Text, ID, Children}`. |
| `Links` | []*Link | Outgoing links to published notes (`Target`, `Embed`), in order, without duplicates. |
| `Backlinks` | []*Backlink | Incoming links: `Source` note and `Context` (text of the linking paragraph). |
| `Folder` | *Folder | |
| `Features` | {Math, Mermaid, Code} | Load KaTeX, Mermaid or highlighting CSS only where needed. |
| `WordCount`, `ReadingTime` | int | Reading time in minutes, at 200 words per minute and at least 1. |

### URLs

A note's URL comes from its path relative to the site's root folder
(`root_folder` in the server config; by default the only always-public
folder, if there is just one): with root folder `Kvist`, `Kvist/My Note.md`
becomes `/my-note/`, and notes outside it keep their full path
(`Reading list.md` → `/reading-list/`). The root folder's own note
(`Kvist/Kvist.md` or `Kvist/index.md`) gets `/`. Each path segment is
lowercased and runs of characters other than letters and digits become `-`.
Unicode letters are kept. Frontmatter `permalink: /about/` overrides the URL.
`index.md`, or a note named like its folder (`Recipes/Recipes.md`), is the
folder's note and gets the folder URL. If two notes would get the same URL,
or a note would take a reserved URL (`/tags/…`, `/_assets/…`,
`/search-index.json`, …), the build fails and names both notes.

### Rendered content

- Links to published notes, written as `[[wikilinks]]` or Markdown links
  (`[text](Note.md)`, resolved from the linking note's folder first):
  `<a class="internal-link" href="…">`. Links to websites:
  `<a class="external-link">`, with dangerous schemes (`javascript:`, …)
  replaced by `#`.
- Links to anything else, private or missing, render the same way:
  `<span class="link-unpublished">text</span>`, so a visitor can't tell a
  private note from a missing one. The text is the alias if there is one,
  else the link as written in the published note.
- Embeds of published notes: `<div class="embed">` with an `embed-title`
  link and `embed-content`. `#Heading` embeds the section, `#^id` the block.
  Embeds of anything unpublished are **omitted entirely**. Cycles and
  embeds nested deeper than 5 levels are omitted.
- Images and other attachments: `/_assets/<hash8>/<name>`. The hash in the
  URL means CDNs may cache them as immutable. Size syntax: `![[a.png|300]]`,
  `![[a.png|300x200]]`. The part after `|` may also hold an alignment and
  alt text, in any order (`![[a.png|A map|right|200]]`); an alignment adds
  `class="align-left|center|right"` to the `<img>`.
- YouTube and Vimeo links written as images (`![title](https://youtu.be/…)`):
  `<iframe class="embed-video">` with the privacy-friendly player
  (`youtube-nocookie.com`, Vimeo with `dnt=1`) and `loading="lazy"`.
- Callouts: `<div class="callout" data-callout="type">` with `callout-title`
  and `callout-content`; foldable ones (`[!tip]-`, `[!tip]+`) use
  `<details>`/`<summary>`.
- Tags: `<a class="tag" href="/tags/…/">`. Control tags (`#public`,
  `#private`) render as `<span class="tag tag-hidden">`.
- `==highlight==` → `<mark>`. Block ids (`^id`) become element ids (`id="^id"`).
- Math: `<span class="math math-inline">\(…\)</span>` and
  `<div class="math math-display">\[…\]</div>`, for KaTeX in the browser.
- Mermaid: `<pre class="mermaid">`, drawn in the browser.
- Code: chroma, with CSS classes (`<div class="code-block" data-lang="go"><pre class="chroma">`).
  A fence's info after the language can ask for a title, line numbers and
  highlighted lines (```` ```go title="main.go" showLineNumbers{10} {2,4-6} ````;
  `linenos` and `hl_lines="2 4-6"` are accepted too). The title is a
  `<div class="code-title">` before the `<pre>`; with numbers or
  highlights each line is `<span class="line">` (`line hl` when
  highlighted) and numbers are `<span class="ln">`. These also work
  without a language, as plain text.
- `%% comments %%` and `<!-- comments -->` are removed before parsing.
- Other HTML written in a note is passed through as written, scripts
  included; links and images inside it are not resolved, so it can't pull
  an unpublished attachment onto the site. It is left out of search text
  and descriptions.
- Embeds of a published canvas (`![[Board.canvas]]`): the canvas's board
  (below) inside the usual `<div class="embed">`.

### Canvases

A canvas (`.canvas`, [JSON Canvas](https://jsoncanvas.org/)) is a `Note`
with `Canvas` set. It is published when it lies in an always-public folder
(and in no excluded one), or when a published note or canvas links to or
embeds it; a canvas in an excluded folder never is. It needs `canvas` in
`attachment_extensions` (in the default list). The `.canvas` file itself is
never output, since it names the vault paths of every card.

- `Title` is the file name, `URL` keeps the extension as a word
  (`Garden/Board.canvas` → `/garden/board-canvas/`), so a note named
  `Board` can sit beside it. `Created` and `Updated` are the file's
  modification time; there are no `Params`, `Aliases` or `TOC`.
- `Tags`, `Links`, `Backlinks`, `Text` (search) and `Description` come from
  the text cards, and the edge and group labels count as text.
- `Content` is `<div class="canvas"><div class="canvas-board" style="width;height">`
  holding the cards, each `<div class="canvas-node canvas-<type>" style="left;top;width;height">`
  (`canvas-text`, `canvas-file`, `canvas-link`, `canvas-group`), placed as
  on the canvas with 40px around them. Groups come first, then
  `<svg class="canvas-edges">` with one `<g class="canvas-edge">` per arrow
  (a curve and `canvas-arrow` heads), then the other cards, then
  `<div class="canvas-edge-label">`s. A preset color adds
  `canvas-color-1` … `canvas-color-6`; a hex color adds `canvas-color` and
  sets `--canvas-color`.
- Text cards are Markdown, rendered and resolved like a note
  (`canvas-node-content`). File cards show a published note as an embed of
  it (`canvas-file-title` and `canvas-node-content`), a published canvas as
  a link, an attachment as its embed. Link cards show http(s) links only.
- A file card pointing to anything unpublished is left out, with the
  arrows to and from it (and their labels), as an embed would be.

## Asset

`Path`, `URL`, `Size`, `MediaType` (MIME), `Hash`. An attachment is
published only if the rendered content of a published note references it,
or it is the image of an image property (a published note's
`image`/`cover`, the settings note's `image`/`favicon`/`avatar`/`logo`), and never if
it is in an `exclude_folders` folder.

## Tag

`Name` (full, e.g. `projects/kvist`), `Slug`, `URL` (`/tags/projects/kvist/`),
`Notes` (listed notes tagged with it or with one of its children, sorted by URL),
`Children`, `Parent`.

## Folder

`Name`, `Path` (the vault path), `URL`, `Notes` (listed notes only),
`Children`, `Index` (the folder note, if any), `Parent`. Only folders that
contain published notes exist. Folders follow the URLs: the root folder is
`Site.Root` (its `Path` is the root folder's vault path), and its subfolders
are the top-level folders. A folder with only unlisted notes, in it and below it, has
`Unlisted` set: it is not among its parent's `Children` and gets no page,
but its notes' `Folder` points to it.

## Graph

`Nodes`: `{ID, Title, URL, Tags}` for every listed note. `Edges`:
`{Source, Target}` for every resolved link between published notes. There
are no nodes for unresolved links. `Tags`: `{Name, URL}` for every tag (as
in `AllTags`), so a theme can draw tags as nodes.

## Generated files

Besides the pages, every build writes these files, all from the model
(published notes only, without unlisted ones):

| File | |
|---|---|
| `/search-index.json` | `{"version": 1, "docs": [{id, title, url, description, tags, aliases, text}]}`, with `text` limited to 20 000 characters per note |
| `/graph.json` | `Graph` as above |
| `/index.xml` | RSS 2.0, the 30 newest notes by `Created`, with title, link, description and tags |
| `/sitemap.xml` | home, notes (with `lastmod`), folders and tags |
| `/robots.txt` | allows everything and points at the sitemap |

## Compatibility

Additions (new fields, new CSS classes) keep version 1. Removing or changing
the meaning of a field bumps `ModelVersion`.
