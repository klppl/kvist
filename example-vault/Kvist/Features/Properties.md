---
stage: budding
created: 2026-09-11
updated: 2026-10-04
tags: [kvist/features]
image: "[[ideas-cover.png]]"
description: Note properties that change how a page looks, from dates to the social preview image.
---
Properties at the top of a note set:

| Property | Shown as |
|---|---|
| `title` | the page title instead of the file name |
| `created`, `updated` | *Planted* and *Tended* dates |
| `stage` | the 🌱 🌿 🌳 badge |
| `description` | the summary in lists, search and link previews |
| `image` or `cover` | the picture when the page is shared |
| `permalink` | a custom address, like [[About]] at `/about/` |
| `aliases` | other names to find and link the note by |

This note's description is set by hand; others use their first paragraph.

Other properties stay private unless the server lists them in `expose_frontmatter`. With `show_properties: true` in the settings note, the exposed ones are listed under the title.
