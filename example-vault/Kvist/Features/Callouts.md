---
stage: evergreen
created: 2026-09-03
tags: [kvist/features]
---
All of Obsidian's callout types, with their colors.

> [!note]
> The default. `> [!note]`

> [!abstract] Abstract, summary, tldr
> For summaries.

> [!info] Info
> Neutral information.

> [!todo] Todo
> - [ ] Callouts can hold task lists.

> [!tip] Tip, hint, important
> Something useful.

> [!success] Success, check, done
> It worked.

> [!question] Question, help, faq
> Is it any good?

> [!warning] Warning, caution, attention
> Careful now.

> [!failure] Failure, fail, missing
> It didn't work.

> [!danger] Danger, error
> Really careful now.

> [!bug] Bug
> Found one.

> [!example] Example
> ```js
> console.log("code inside a callout");
> ```

> [!quote] Quote, cite
> Words someone else said.

## Folding

> [!tip]- Folded: click to open
> Starts closed. Written as `[!tip]-`.

> [!tip]+ Foldable, starts open
> Written as `[!tip]+`.

## Nesting

> [!info] Outer
> Callouts can nest.
> > [!warning] Inner
> > Like this.

## Custom titles

> [!success] Bread came out well
> A title after the type replaces the default one.
