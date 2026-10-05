# todo

Features compared against Quartz and Obsidian Publish. Each item says which
of them has it today.

## Need to have

- [x] **Favicon.** A `favicon:` in `_site.md` pointing to an image in the
  vault, with a default icon from the theme. Today the pages have no
  `<link rel="icon">`. (Quartz, Publish)
- [x] **Redirects for moved notes.** Renaming or moving a note changes its
  address and breaks old links. The server keeps earlier revisions, so it
  can record the old addresses and redirect them, and a `redirect_from:`
  property can cover the rest. (Quartz turns `aliases` into redirect pages)
- [x] **Folder tree in the menu.** The menu lists only top-level folders,
  so notes deeper down are reached only through the middle list. A
  collapsible tree, with the current note's folder open. (Quartz Explorer,
  Publish navigation)
- [x] **Translated interface.** `language` only sets page metadata; words
  like "Planted", "Tended", "Links to this note" and "Search" stay in
  English. Start with English and Swedish, in a file a theme can extend.
  (Quartz has about 30 languages)
- [x] **Analytics setting.** A choice of privacy-friendly analytics
  (Plausible, Umami, GoatCounter) by domain or site ID, so no one needs a
  theme override for it. (Quartz, Publish with Google Analytics)
- [x] **Copy button on code blocks.** (Quartz)

## Nice to have

- [ ] **Canvas.** Render `.canvas` files as a page with cards and arrows,
  and embeds with `![[board.canvas]]`. (Publish)
- [ ] **Instant navigation.** Load the next page without a full reload,
  keeping the menu's scroll position and the graph. (Quartz)
- [ ] **Breadcrumbs.** The note's folders above its title. (Quartz)
- [ ] **Reader mode.** A button that hides the menu, list and side panels.
  (Quartz)
- [ ] **Stacked pages.** Open linked notes side by side, as panes that
  slide. (Publish)
- [ ] **Password-protected sites.** One password for the whole site, or
  for a folder. (Publish)
- [ ] **Generated social images.** A preview image with the note's title
  and the site's name, for notes without `image:`. (Quartz)
- [ ] **Comments.** An optional giscus block under notes. (Quartz)
- [ ] **Code block extras.** A title (```` ```go title="main.go" ````),
  line numbers and highlighted lines. (Quartz)
- [ ] **Custom CSS from the vault.** A `publish.css` in the vault, applied
  after the theme's styles, so small style changes don't need the server.
  (Publish)
- [ ] **Order and hide menu entries.** Choose the order of folders and
  groups, and hide some from the menu without unpublishing them. (Publish)
- [ ] **Unlisted notes.** `unlisted: true` publishes a note but leaves it
  out of lists, search, the graph, feeds and the sitemap. Also `noindex`
  for search engines.
- [ ] **Search by tag.** `#tag` in the search box filters the results.
  (Quartz)
- [ ] **Citations.** `[@key]` resolved from a BibTeX file in the vault,
  with a bibliography at the end. (Quartz)
- [ ] **Sites under a sub-path.** `base_url` like
  `https://example.com/garden/`, which the docs say isn't supported yet.
  (Quartz)
- [ ] **Excalidraw.** Show drawings from their exported SVG or PNG.
  (Publish, with the Excalidraw plugin's export)

## Fixes found along the way

- [ ] **Docs: Markdown links work.** `docs/writing.html` says links like
  `[text](Other%20note.md)` aren't converted, but they resolve fine,
  including `#Heading`.
- [ ] **Docs: aliases work as link targets.** `[[Another name]]` links to
  the note with that alias; the docs say it doesn't.
- [ ] **Docs: block embeds.** `![[Note#^id]]` works but isn't in the table
  of what works.
- [ ] **theme.toml comment.** The comment for `show_properties` ended up at
  the end of the `show_profile` line in `themes/garden/theme.toml`.
