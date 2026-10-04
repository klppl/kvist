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
| `Assets` | []*Asset | Attachments referenced by rendered content, sorted by URL. |
| `Tags` | []*Tag | Top-level tags; nested tags are `Children`. |
| `AllTags` | []*Tag | Every tag, sorted by name (templates only). |
| `Root` | *Folder | Folder tree of published notes. |
| `Graph` | Graph | Nodes and edges between published notes. |
| `Home` | *Note | The root `index.md`, or `home` from the settings note; nil means the theme generates an index. |

## SiteConfig

`Title`, `Description`, `BaseURL` (no trailing slash), `Language`, `Author`,
`StrictLineBreaks` (single line breaks were rendered as spaces, not `<br>`),
`Nav` (list of `{Title, URL}`) and `Params` (theme parameters).

The server config sets these. The vault may override `title`,
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
are a single link: URLs are kept, `[[wikilinks]]` resolve to published notes
(links to anything else are dropped with a warning). Links in comments and
code blocks don't count.

The older `.kvist/site.toml` is still read (`title`, `description`,
`author`, `language`, `home`, `[[nav]]`, `[theme_params]`); the settings
note wins where both set a value. The theme parameter `nav_tags` is read as
`groups`.

## Note

| Field | Type | |
|---|---|---|
| `ID` | string | URL without slashes (`garden/my-note`); `index` for `/`. |
| `Path` | string | Vault path (`Kvist/My Note.md`). |
| `URL` | string | `/kvist/my-note/`. |
| `Slug` | string | URL without surrounding slashes. |
| `Title` | string | Frontmatter `title`, else the first `# Heading`, else the file name. |
| `Aliases` | []string | Frontmatter `aliases`. |
| `Tags` | []*Tag | Frontmatter tags plus tags in the body, without the control tags and without tags inside comments. |
| `Created`, `Updated` | time | Frontmatter `created`/`date` and `updated`/`modified`/`lastmod`, else the file's modification time. |
| `Description` | string | Frontmatter `description`/`summary`, else the first paragraph (≤ 200 characters). |
| `Params` | map | Only the frontmatter keys listed in `expose_frontmatter`. Other keys never reach a theme. |
| `Content` | HTML | The rendered body. |
| `TOC` | []*Heading | Nested `{Level, Text, ID, Children}`. |
| `Links` | []*Link | Outgoing links to published notes (`Target`, `Embed`), in order, without duplicates. |
| `Backlinks` | []*Backlink | Incoming links: `Source` note and `Context` (text of the linking paragraph). |
| `Folder` | *Folder | |
| `Features` | {Math, Mermaid, Code} | Load KaTeX, Mermaid or highlighting CSS only where needed. |
| `WordCount`, `ReadingTime` | int | Reading time in minutes, at 200 words per minute and at least 1. |

### URLs

`Kvist/My Note.md` becomes `/kvist/my-note/`. Each path segment is
lowercased and runs of characters other than letters and digits become `-`.
Unicode letters are kept. Frontmatter `permalink: /about/` overrides the URL.
`index.md`, or a note named like its folder (`Recipes/Recipes.md`), is the
folder's note and gets the folder URL. If two notes would get the same URL,
or a note would take a reserved URL (`/tags/…`, `/_assets/…`,
`/search-index.json`, …), the build fails and names both notes.

### Rendered content

- Links to published notes: `<a class="internal-link" href="…">`.
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
- `%% comments %%` and `<!-- comments -->` are removed before parsing.

## Asset

`Path`, `URL`, `Size`, `MediaType` (MIME), `Hash`. An attachment is
published only if the rendered content of a published note references it,
and never if it is in an `exclude_folders` folder.

## Tag

`Name` (full, e.g. `projects/kvist`), `Slug`, `URL` (`/tags/projects/kvist/`),
`Notes` (notes tagged with it or with one of its children, sorted by URL),
`Children`, `Parent`.

## Folder

`Name`, `Path`, `URL`, `Notes`, `Children`, `Index` (the folder note, if any),
`Parent`. Only folders that contain published notes exist.

## Graph

`Nodes`: `{ID, Title, URL, Tags}` for every published note. `Edges`:
`{Source, Target}` for every resolved link between published notes. There
are no nodes for unresolved links.

## Generated files

Besides the pages, every build writes these files, all from the model
(published notes only):

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
