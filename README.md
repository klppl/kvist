# kvist

kvist (Swedish for "twig") publishes selected notes from an Obsidian vault as
a themeable static website: a digital garden. The
[Obsidian plugin](plugin/README.md) decides what is public and pushes it,
from desktop or mobile (`kvist push` does the same from a terminal). A
self-hosted Go server checks the publish rules again, builds the site with
the built-in garden theme, publishes it atomically and serves it. The site's
title, menu and look are set in a note in your vault, `_site.md`, so you can
change them from Obsidian.

**Status:** feature-complete, but not yet run in production.

**[Read the documentation →](https://klppl.github.io/kvist/)** It walks you
through setting up a server, connecting Obsidian and publishing your first
notes.

## Run on a server

Every push to `main` publishes a Docker image, `ghcr.io/klppl/kvist`
(linux/amd64). On a VPS with Docker, put
[`docker-compose.yml`](deploy/docker-compose.yml), the
[`Caddyfile`](deploy/Caddyfile) and [`kvist.toml`](deploy/kvist.toml) in a
folder, set your domain, and run:

```sh
docker compose up -d
docker compose exec kvist kvist token create --site garden --name laptop
```

Caddy handles HTTPS. [Getting started](https://klppl.github.io/kvist/getting-started.html)
walks through it, including the Obsidian plugin. [Deploying](docs/deploy.md)
also covers systemd without Docker, nginx and Cloudflare.

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
`kvist token list|revoke`, `kvist rollback SITE REVISION`, `kvist gc`, and
`kvist build --dir example-vault --out site` to build a site from a folder
without a server (`--emit-model model.json` shows what a theme receives).

With `serve = true` in a site's config, `kvist serve` also serves the built
site for requests to the site's host. With a single served site, any host
works, so `http://127.0.0.1:8080/` shows it.

## Preview locally

```sh
./kvist dev --dir ~/Vault --config kvist.toml   # same rules and theme as the server
./kvist dev --dir ~/Vault --public Kvist       # without a config
./kvist dev --dir example-vault --config kvist.example.toml   # the demo vault
```

This serves the site at http://127.0.0.1:1313/ and rebuilds when a note or
theme file changes. The browser reloads by itself, and build errors show at
the bottom of the page while the last good build stays up.

## What gets published

The server config defines the rules. Per note, the first match wins:

0. The settings note, `_site.md`, is never a page.
1. Notes in an `exclude_folders` folder stay private.
2. `publish: false` in the frontmatter, or a `#private` tag, keeps a note private.
3. Notes in an `always_public_folders` folder, with a `#public` tag, or with
   `publish: true` are published.
4. Everything else stays private.

An attachment is published only if a published note links to or embeds it,
and never if it is in an excluded folder.
The client applies the rules before uploading anything, and the server
applies them again on its own.

## Docs

The user guide is at **https://klppl.github.io/kvist/** (source: `docs/*.html`,
served by GitHub Pages).
Reference notes for developers:

- [Design](docs/design.md): architecture, decisions, threat model
- [Sync protocol v1](docs/protocol.md): the client–server contract
- [Content model v1](docs/content-model.md): what themes receive
- [Themes](docs/themes.md): writing and customizing themes
- [Deploying](docs/deploy.md): Docker or systemd, Caddy/nginx, Cloudflare
- [Obsidian plugin](plugin/README.md)
- [Contributing](CONTRIBUTING.md)

## Development

```sh
make test     # go vet + go test -race ./...
```

Requires Go 1.26 or later (and Node 20+ for the plugin). MIT licensed.
