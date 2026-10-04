# Themes

A theme is a folder that turns the [content model](content-model.md) into
HTML. kvist ships with **garden**, which is built into the binary. A theme
only sees published notes, so it cannot leak a private one.

## Choosing and customizing a theme

```toml
[[site]]
theme           = "garden"            # built-in, a folder in themes_dir, or a path
theme_overrides = "/etc/kvist/garden-overrides"   # optional

  [site.theme_params]                 # override the theme's [params]
  accent = "#8a4f9e"
```

`themes_dir` (top level, default `themes`) is searched before the built-in
themes, so `themes/garden/` replaces the built-in garden.

`theme_overrides` is a folder with the same layout as a theme. Any file in
it replaces the theme's file with the same path, for example
`templates/partials/note-parts.html` or `static/style.css`. This lets you
change one template without forking the theme.

The garden theme includes an empty `templates/partials/head-extra.html` at
the end of `<head>`. Override it to add a stylesheet (after the theme's
own), a script or meta tags without copying any other file:

```html
{{define "head-extra" -}}
<link rel="stylesheet" href="{{asset "custom.css"}}">
{{- end}}
```

with `static/custom.css` in the same overrides folder.

Theme parameters can also be set from the vault, as properties of the
settings note (`_site.md`); see the [content model](content-model.md#siteconfig).
The vault's values win.

## Layout

```
mytheme/
  theme.toml
  templates/
    base.html          the page skeleton; pages fill {{block "main"}} and {{block "aside"}}
    home.html          /
    note.html          every note
    folder.html        folders without a folder note
    tag.html           /tags/<tag>/
    tags.html          /tags/
    404.html           /404.html
    partials/*.html    shared {{define}} blocks
  static/              copied to /_kvist/<hash>/
```

```toml
# theme.toml
name    = "mytheme"
version = "1.0.0"
license = "MIT"
model   = [1]          # content model versions this theme supports

[params]               # defaults; sites override them with theme_params
accent = "#3f7d4e"
```

kvist refuses a theme whose `model` list doesn't include the version it
produces.

## Templates

Templates use Go's `html/template`, which escapes output by context. They
cannot read files or make network requests. Each page template is parsed
together with `base.html` and the partials, and `base.html` is executed with
a `Page`:

| Field | |
|---|---|
| `.Kind` | `home`, `note`, `folder`, `tag`, `tags` or `404` |
| `.Site` | the whole content model (`.Site.Notes`, `.Site.Config.Title`, `.Site.Root`, …) |
| `.Title`, `.URL`, `.Description` | for `<title>`, canonical links and meta tags |
| `.Note` | the note (note pages; home when a home note exists) |
| `.Folder` | folder pages |
| `.Tag` | tag pages |
| `.Features` | `{Math, Mermaid, Code}`: load scripts and styles only where needed |
| `.Theme` | name and version |

### Functions

| Function | Example | |
|---|---|---|
| `asset` | `{{asset "style.css"}}` | URL of a file in `static/`, content-hashed |
| `absURL` | `{{absURL .URL}}` | absolute URL with the site's base URL |
| `dateFormat` | `{{dateFormat "2 Jan 2006" .Note.Updated}}` | Go time layout |
| `isoDate` | `{{isoDate .Note.Created}}` | RFC 3339 |
| `param` | `{{param "accent"}}` | site `theme_params` over theme defaults |
| `recent` | `{{range recent 10}}` | most recently updated notes |
| `newest` | `{{range newest 3 .Tag.Notes}}` | the given notes, most recently updated first (`0` for all) |
| `markdownify` | `{{markdownify (str (param "footer"))}}` | Markdown → HTML (internal links are not resolved) |
| `truncate` | `{{truncate 160 .Description}}` | at a word boundary |
| `default` | `{{default "Untitled" .Title}}` | |
| `dict` | `{{template "x" (dict "A" 1 "B" .)}}` | pass several values |
| `json` | `<script>var d = {{json .Site.Graph}}</script>` | |
| `propertyValues` | `{{range propertyValues (param "date_format") .Note.Params.authors}}` | a property as display strings: one per list item, dates in the layout, `[[links]]` as their text |
| `lower`, `upper`, `title`, `join`, `replace`, `contains`, `hasPrefix`, `add`, `sub`, `str` | | |

Code highlighting styles come from the `code_style_light` and
`code_style_dark` params ([chroma style names](https://xyproto.github.io/splash/docs/)).
They are written to `syntax.css` (`{{asset "syntax.css"}}`).

## Output and caching

| Path | Cache-Control (built-in server) |
|---|---|
| pages (`/kvist/note/index.html`, …) | `public, max-age=60` |
| `/_assets/<hash>/…` (attachments) | `public, max-age=31536000, immutable` |
| `/_kvist/<hash>/…` (theme files) | `public, max-age=31536000, immutable` |

HTML comments are removed from every page.

## The garden theme

Params (set them in `theme_params`):

| Param | Default | |
|---|---|---|
| `accent` | `#3f7d4e` | link and highlight color |
| `date_format` | `2 Jan 2006` | Go time layout |
| `home_recent` | `12` | notes in the home page's list |
| `show_list` | `true` | the list pane (below) |
| `list_max` | `100` | notes in the list pane before a "more…" link |
| `groups` | `[]` | tags that group the sidebar instead of folders (below); `nav_tags` is an older name for it |
| `show_toc`, `show_backlinks`, `show_graph` | `true` | note page parts |
| `show_properties` | `false` | the note's properties under its title; only keys listed in `expose_frontmatter` exist, so nothing new reaches the page |
| `code_style_light`, `code_style_dark` | `github`, `github-dark` | highlighting |
| `footer` | `""` | Markdown at the bottom of the menu |
| `mermaid_url`, `mermaid_integrity` | jsDelivr, pinned with an SRI hash | see below |

Pages have three panes: the nav pane (main links, the top-level folders or
`groups`, and the menu links from the settings note), the list pane, and
the page. The list pane shows
the notes around the page: a note's folder, a folder's notes, a tag's notes,
or the most recent notes on the home page. It has a filter box, and on
small screens it moves below the page. Neither pane lists the whole vault,
and the list stops at `list_max`, because a full tree on every page made a
5 000-note site 2 GB. On wide screens the table of contents and the local
graph sit to the right of a note; otherwise they follow it.

**Grouping by tags.** By default the sidebar lists your top-level folders.
If everything you publish lives in one folder, group it by tags instead:

```markdown
---
groups: [articles, projects, books]
---
```

(in the settings note, `_site.md`)

Each tag becomes a sidebar entry, in this order, labeled with a capital
first letter ("Articles") and linked to its tag page; notes with a child
tag (`#projects/kvist`) count too. On a note page, the list pane shows the
note's group, the first `groups` tag it carries, and falls back to its
folder for notes without one. Tags with no published notes are left out.

It shows the growth stage from the frontmatter `stage` (`seedling`,
`budding`, `evergreen`) when `stage` is listed in `expose_frontmatter`.

Hovering a heading in a note shows a `#` next to it; clicking it copies a
link to that section.

Printing a page prints only the note, in light colors: the panes, the
table of contents, the graph and backlinks are left out, folded callouts
are opened, and external links show their address.

Search (press <kbd>/</kbd>) loads MiniSearch and `/search-index.json` the first time
it opens. The graph (the Graph item in the menu, and the "Connections" box on note pages)
reads `/graph.json` and draws on a canvas. KaTeX is vendored and loads only
on pages with math.

**Mermaid** is about 5 MB, so it is not bundled. Pages that contain a
diagram load it from jsDelivr, pinned to one version and checked with a
subresource-integrity hash. To avoid the third-party request, put
`mermaid.min.js` in your `theme_overrides` folder as
`static/vendor/mermaid.min.js` and set:

```toml
[site.theme_params]
mermaid_url       = "vendor/mermaid.min.js"   # a theme file, or an https:// or /absolute URL
mermaid_integrity = ""                        # or the file's sha384 hash
```

Without JavaScript, math and diagrams show their source.

## What a theme should do

- Work without JavaScript. Use scripts for enhancements only (search,
  graph, theme toggle).
- Style `.link-unpublished` as plain text. It covers both private and missing
  notes, so don't style it to suggest that something hidden exists.
- Load KaTeX and Mermaid only when `.Features.Math` / `.Features.Mermaid`
  are set.
- Not fetch anything from third-party hosts: vendor fonts and libraries in
  `static/`.
