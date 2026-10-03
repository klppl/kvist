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

`theme_params` can also be set from the vault in `.kvist/site.toml`.

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
| `markdownify` | `{{markdownify (str (param "footer"))}}` | Markdown → HTML (internal links are not resolved) |
| `truncate` | `{{truncate 160 .Description}}` | at a word boundary |
| `default` | `{{default "Untitled" .Title}}` | |
| `dict` | `{{template "x" (dict "A" 1 "B" .)}}` | pass several values |
| `json` | `<script>var d = {{json .Site.Graph}}</script>` | |
| `lower`, `upper`, `title`, `join`, `contains`, `hasPrefix`, `add`, `sub`, `str` | | |

Code highlighting styles come from the `code_style_light` and
`code_style_dark` params ([chroma style names](https://xyproto.github.io/splash/docs/)).
They are written to `syntax.css` (`{{asset "syntax.css"}}`).

## Output and caching

| Path | Cache-Control (built-in server) |
|---|---|
| pages (`/garden/note/index.html`, …) | `public, max-age=60` |
| `/_assets/<hash>/…` (attachments) | `public, max-age=31536000, immutable` |
| `/_kvist/<hash>/…` (theme files) | `public, max-age=31536000, immutable` |

HTML comments are removed from every page.

## What a theme should do

- Work without JavaScript. Use scripts for enhancements only (search,
  graph, theme toggle).
- Style `.link-unpublished` as plain text. It covers both private and missing
  notes, so don't style it to suggest that something hidden exists.
- Load KaTeX and Mermaid only when `.Features.Math` / `.Features.Mermaid`
  are set.
- Not fetch anything from third-party hosts: vendor fonts and libraries in
  `static/`.
