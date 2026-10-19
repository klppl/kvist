---
stage: budding
created: 2026-10-19
tags: [kvist/features]
layout: cover
image: "[[garden-cover.png]]"
description: Five ways to lay out a page, picked with one property.
---
A `layout` property picks how a page is laid out. This note uses `layout: cover`, so its image runs wide above the title.

| Layout | For | Shows |
|---|---|---|
| `article` | notes (the default) | folders, title, dates, tags, backlinks, related notes |
| `cover` | posts and projects | the `image` or `cover` wide on top, then like an article |
| `page` | pages like [[Now]], contact or colophon | the title and the description, nothing else |
| `about` | a page about you, like [[About]] | a portrait, the title, the description and your profile links |
| `intro` | the home note, like [[Welcome|Welcome to the garden]] | the image as a banner, a large title and the description |

## Writing the description

The `page`, `cover`, `about` and `intro` layouts show the `description` property under the title, as a lead. Write it as one or two short sentences. Without one, the note's first paragraph is the lead on an `intro` page.

## Pictures

- `cover` and `intro` show the note's `image` or `cover`. A wide picture works best: it is cropped to 2:1 (5:2 for `intro`), and closer to 16:10 on phones.
- `about` shows the note's image as a portrait, cropped to 4:5 around the upper middle. Without one, it shows the `avatar` from the settings note.
- `intro` without an image shows the avatar, small and round, above the title.

## What is left out

`page`, `about` and `intro` are pages of their own, not notes in the garden: they leave out the dates, the stage, tags, backlinks, related notes and the local graph. They still have a table of contents when they have headings.

In Obsidian, the kvist plugin's *Set page layout* command adds the property for you.
