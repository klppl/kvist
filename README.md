# kvist

kvist (Swedish for "twig") publishes selected notes from an Obsidian vault as
a themeable static website: a digital garden. An Obsidian plugin decides what
is public and pushes it. A self-hosted Go server checks it again, stores it,
builds the site and serves it.

**Status: early development.** Phase 1 of [the plan](docs/design.md#11-phases)
is done: the server stores pushed content and enforces the publish rules,
but it does not render a website yet.

| Phase | | Status |
|---|---|---|
| 1 | Config, content store, sync protocol and API, tokens, `kvist push` | done |
| 2 | Markdown, link resolution, content model, leak tests | next |
| 3 | Renderer, themes, garden theme, atomic builds, static serving | |
| 4 | Search, graph, backlinks, RSS, sitemap, attachments | |
| 5 | Dev server with live reload, incremental builds | |
| 6 | Obsidian plugin | |
| 7 | Docker, deployment guide, Cloudflare | |

## Try it

```sh
go build -o kvist ./cmd/kvist

# Server
cp kvist.example.toml kvist.toml           # set data_dir and the site
./kvist token create --site garden --name laptop   # prints the token once
./kvist serve

# Client (in another shell)
export KVIST_TOKEN=kvist_…
./kvist push --server http://127.0.0.1:8080 --site garden --dir example-vault -v
```

`-v` lists the publish decision for every note. Other commands:
`kvist token list|revoke`, `kvist rollback SITE REVISION`, `kvist gc`.

## What gets published

The server config defines the rules. Per note, the first match wins:

1. Notes in an `exclude_folders` folder stay private.
2. `publish: false` in the frontmatter, or a `#private` tag, keeps a note private.
3. Notes in an `always_public_folders` folder, with a `#public` tag, or with
   `publish: true` are published.
4. Everything else stays private.

An attachment is published only if a published note links to or embeds it.
The client applies the rules before uploading anything, and the server
applies them again on its own.

## Docs

- [Design](docs/design.md): architecture, decisions, threat model
- [Sync protocol v1](docs/protocol.md): the client–server contract

## Development

```sh
make test     # go vet + go test -race ./...
```

Requires Go 1.24.
