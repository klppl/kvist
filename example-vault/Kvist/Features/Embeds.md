---
stage: evergreen
created: 2026-09-07
tags: [kvist/features]
---
Embeds show another note, or part of one, inside this one.

## A whole note

`![[Learning in public]]`

![[Learning in public]]

## A section

`![[Soil#Texture]]`

![[Soil#Texture]]

## A block

`![[Soil#^add-compost]]` embeds one paragraph that has a block id:

![[Soil#^add-compost]]

## A PDF

`![[seed-packet.pdf]]`

![[seed-packet.pdf]]

## Private notes are left out

`![[Diary]]` is below this line, but [[Diary]] is private, so the embed is left out completely:

![[Diary]]

…and nothing is shown.
