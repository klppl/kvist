---
stage: budding
created: 2026-10-06
tags: [kvist/features]
---
A canvas (a `.canvas` file) is published as a page of its own, with its cards and arrows where you put them. Text cards are Markdown, file cards show the note or image they point to, and a card showing an unpublished note is left out, with its arrows.

A canvas in an always-public folder is published by itself; any other canvas is published when a published note links to it, like an attachment. Link to one with `[[Garden board.canvas]]`, or embed it with `![[Garden board.canvas]]`:

![[Garden board.canvas]]
