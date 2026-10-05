---
stage: budding
created: 2026-09-08
tags: [kvist/features, meta]
---
Tags can be written in the text, like #garden or #kitchen/bread, or in the note's properties (`tags: [kvist/features, meta]`).

- Each tag has its own page, and the **Tags** page in the menu lists them all.
- Nested tags like #garden/soil also count for their parent, #garden.
- Tags inside code, like `#notatag`, are not tags.
- The control tags `#public` and `#private` are hidden from the site.

> [!warning] A tag in a sentence is a real tag
> Mentioning a control tag in plain text adds it to the note. When you only mean to talk about it, write it as code: `#private`.
