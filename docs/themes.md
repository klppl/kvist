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

The garden theme's `templates/partials/analytics.html` loads the site's
analytics script (`Site.Config.Analytics`) in `<head>`.

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
  i18n/<lang>.toml     the theme's words per language (optional)
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
| `.SocialImage` | note pages without an image property, when the theme's `social_images` param is `true`: the URL of a generated 1200×630 PNG with the title, description and site name (below) |
| `.Theme` | name and version |

### Functions

| Function | Example | |
|---|---|---|
| `asset` | `{{asset "style.css"}}` | URL of a file in `static/`, content-hashed |
| `t` | `{{t "search"}}`, `{{t "notes" (len .Notes)}}` | a word in the site's language (below) |
| `i18n` | `<body data-i18n="{{json (i18n "script")}}">` | a table of words, e.g. for a script |
| `absURL` | `{{absURL .URL}}` | absolute URL with the site's base URL |
| `dateFormat` | `{{dateFormat "2 Jan 2006" .Note.Updated}}` | Go time layout; month and day names in the site's language |
| `isoDate` | `{{isoDate .Note.Created}}` | RFC 3339 |
| `param` | `{{param "accent"}}` | site `theme_params` over theme defaults |
| `recent` | `{{range recent 10}}` | most recently updated notes, without unlisted ones (`0` for all) |
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

## Translations

A theme's words live in `i18n/<lang>.toml`, one file per language code,
and templates show them with `t`. The site's `language` picks the file:
`sv-SE` tries `sv-se.toml`, then `sv.toml`, then `en.toml`; a word that is
still missing shows as its key.

```toml
# i18n/sv.toml
search = "Sök"
notes  = { one = "%d anteckning", other = "%d anteckningar" }  # {{t "notes" 3}}
months = ["januari", "februari", …]   # also months_short, days, days_short (Sunday first)

[script]                              # words for the theme's script
close = "Stäng"
```

A table with `one` and `other` is a plural: the first argument picks
`one` for 1 and `other` for anything else, and arguments fill `%d` and
`%s`. A file in `theme_overrides` adds to the theme's file of the same
name key by key, so a site can change one word or add a language without
copying the rest. The garden theme ships `en`, `sv`, `de`, `fr` and `es`;
a test keeps their keys in step with `en.toml`.

## Output and caching

| Path | Cache-Control (built-in server) |
|---|---|
| pages (`/note/index.html`, …) | `public, max-age=60` |
| `/_assets/<hash>/…` (attachments) | `public, max-age=31536000, immutable` |
| `/_kvist/<hash>/…` (theme files) | `public, max-age=31536000, immutable` |
| `/_kvist/social/<hash>.png` (social images) | `public, max-age=31536000, immutable` |

HTML comments are removed from every page.

A theme whose `[params]` has `social_images = true` gets a preview image
for every note page without an image property (not the home page): the
site's name with a dot in the `accent` color, the title (as large as fits
in three lines), the description and the site's domain, drawn with the Go
fonts. Notes whose text those fonts can't show (Chinese, Japanese, …) get
none. Images are named by a hash of what they show, so unchanged ones are
reused from the previous build.

## The garden theme

Params (set them in `theme_params`):

| Param | Default | |
|---|---|---|
| `accent` | `#3f7d4e` | link and highlight color |
| `date_format` | `2 Jan 2006` | Go time layout |
| `home_recent` | `12` | notes in the home page's list |
| `show_list` | `true` | the list pane (below) |
| `list_max` | `100` | notes in the list pane before a "more…" link |
| `groups` | `[]` | tags that group the sidebar instead of folders (below) |
| `show_toc`, `show_backlinks`, `show_graph` | `true` | note page parts |
| `show_breadcrumbs` | `true` | the folders above a note's title (and a folder's), each linking to its folder page |
| `graph_orphans` | `true` | the global graph shows notes without links |
| `graph_tags` | `false` | the global graph shows tags as nodes, linked to their notes (nested tags to their parent) |
| `graph_node_size` | `links` | `links`: nodes grow with their number of links; `same`: all nodes alike |
| `graph_link_distance` | `45` | how long links are at rest, in pixels; larger spreads the graph out |
| `instant_navigation` | `true` | links to the site's pages load in place: the page, its head metadata and the list pane (kept when it lists the same notes) are swapped, the nav pane keeps its scroll and open folders, search and the graph stay loaded, and scripts and styles a page needs (KaTeX, Mermaid) are added; anything unexpected falls back to a normal load |
| `link_previews` | `true` | hovering a link to a note shows a preview of it (of the section, for `#heading` links); on touch screens the first tap previews and the second opens |
| `reader_mode` | `true` | a button in the page's top corner that hides the nav pane, the list pane and the aside (wide screens only); the reader's choice is kept in `localStorage` |
| `show_profile` | `true` | the profile (`Site.Config.Profile`: avatar, author, bio, links) at the top of the menu |
| `show_properties` | `false` | the note's properties under its title; only keys listed in `expose_frontmatter` exist, so nothing new reaches the page |
| `social_images` | `true` | generate `.SocialImage` for notes without an `image`; the `og:image` is the note's image, else this, else the site's |
| `code_style_light`, `code_style_dark` | `github`, `github-dark` | highlighting |
| `footer` | `""` | Markdown at the bottom of the menu |
| `mermaid_url`, `mermaid_integrity` | jsDelivr, pinned with an SRI hash | see below |
| `analytics`, `analytics_id`, `analytics_url` | none | visitor statistics; kvist checks them and themes get `Site.Config.Analytics` ([content model](content-model.md#siteconfig)) |

Pages have three panes: the nav pane (main links, the folder tree or
`groups`, and the menu links from the settings note), the list pane, and
the page. The list pane shows
the notes around the page: a note's folder, a folder's notes, a tag's notes,
or the most recent notes on the home page. It has a filter box, and on
small screens it moves below the page. The folder tree holds folders, not
notes: folders on the way to the page start open, and so does the only
top-level folder of a garden that has one. Neither pane lists every note,
and the list stops at `list_max`, because a full tree of notes on every
page made a 5 000-note site 2 GB. On wide screens the table of contents and the local
graph sit to the right of a note; otherwise they follow it.

**Grouping by tags.** By default the sidebar shows your folders, from the
site's root folder down (`Site.Root`). If your notes aren't sorted into
folders, group them by tags instead:

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
it opens. Words starting with `#` filter the results to notes with a tag
that starts with them (`#garden` also matches `#garden/soil`); with only
tags, it lists every such note. The graph (the Graph item in the menu, and the "Connections" box on note pages)
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
