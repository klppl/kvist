# todo

- [x] **Link previews on hover.** Show a popup with the linked note when
  hovering an internal link. On touch screens, a tap opens the preview.
  Only published notes, so it can reuse the rendered pages.
- [x] **Video embeds.** `![title](https://youtube.com/watch?v=…)` and Vimeo
  links become an embedded player (privacy-friendly `youtube-nocookie.com`,
  `loading="lazy"`).
- [x] **Image alignment.** `![[img.png|center]]`, `![[img.png|right|200]]`,
  and alt text with size: `![[img.png|Description|100]]`.
- [x] **Social preview image.** `og:image` and Twitter card tags: a site
  default in `_site.md`, overridden per note by an `image:` or `cover:`
  property.
- [x] **Copy link button on headings.** A small anchor next to each heading
  that copies the section URL.
- [x] **Print stylesheet.** `@media print`: hide the sidebars, menu and graph,
  print only the note, in light colors.
- [x] **Show note properties.** A properties block at the top of the note,
  opt-in per theme setting. Only keys listed in `expose_frontmatter` are
  shown, so nothing new reaches the page by default.
- [x] **Graph options.** Show or hide orphans, tags as nodes, node size by
  number of links, link distance.
- [x] **Line breaks setting.** Make hard wraps optional (Obsidian's "strict
  line breaks"); today single line breaks always become `<br>`.
- [x] **Profile block.** An optional slot with a logo or avatar and links
  (GitHub, Mastodon, …), set in `_site.md`.
