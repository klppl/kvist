---
stage: budding
created: 2026-09-10
updated: 2026-10-01
aliases: [publishing]
tags: [meta, kvist]
---
kvist decides per note whether it is public. The server holds the rules and
checks every note again, even though the plugin already did.

## The rules

1. Notes in an excluded folder stay private.
2. `publish: false` or a `#private` tag keeps a note private.
3. Notes in an always-public folder, with a `#public` tag or with
   `publish: true` are published.
4. Everything else stays private. ^default-private

## Attachments

An attachment is published only when a published note links to or embeds
it. Unused files stay home.

%% This comment never leaves the vault. %%
