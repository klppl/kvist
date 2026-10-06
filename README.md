# kvist

[![100% vibecoded](https://img.shields.io/badge/vibecoded-100%25-ff69b4?logo=sparkles&logoColor=white)](#)
[![Made with love](https://img.shields.io/badge/made%20with-%E2%9D%A4%EF%B8%8F%20%26%20a%20suspicious%20eye-blueviolet)](#)
[![Code reviewed by](https://img.shields.io/badge/code%20reviewed%20by-an%20actual%20human-success)](#)

> 🌿 **Disclosure:** every line of kvist was vibecoded. Lovingly. By someone
> who knows what a race condition is, reads the diffs, and has said "no,
> that's wrong, try again" more times than they've said "ship it". The vibes
> are immaculate; the tests still have to pass.

**Publish your Obsidian notes as a website you own.**

kvist (Swedish for "twig") turns the notes you choose into a fast,
searchable digital garden on your own server. Write in Obsidian, press
Publish, and your site updates. Private notes never leave your device.

![A kvist site: a menu with a profile and folder tree, a list of the notes beside it, and a note with callouts and code](docs/assets/screens/note-light.webp)

- **Obsidian syntax:** wikilinks, embeds, callouts, tags, footnotes, math,
  Mermaid diagrams and canvases.
- **Private stays private:** you choose what's public by folder, tag or
  property, and the server checks every note again. Links to private notes
  become plain text.
- **Your folder is the site:** publish one folder from your vault and it
  becomes the site's root, so its subfolders are the menu and the folder's
  name stays out of every address.
- **Search, backlinks and a graph,** in a three-pane theme with light and
  dark mode, breadcrumbs, a reader mode, instant page loads and optional
  stacked pages that open linked notes side by side. Search filters by
  `#tag`.
- **Shares well:** every note gets a preview image with its title for
  chats and social media, and `unlisted: true` publishes a note without
  listing it anywhere.
- **Settings in a note:** the site's title, menu and colors live in
  `_site.md`, so you change them from Obsidian on any device.
- **Links keep working:** a renamed or moved note's old address redirects
  to the new one.
- **In your language:** the theme's menus, dates and search speak English,
  Swedish, German, French or Spanish.
- **Self-hosted:** one small Go server in Docker, with a ready-made Caddy
  setup for HTTPS and room for several sites. RSS,
  a sitemap, photo metadata stripping and optional cookie-free analytics
  included.

## TLDR

You need a server with Docker, and an HTTPS address that forwards to
`127.0.0.1:8080` on it (Cloudflare, nginx, Caddy, whatever you use). On the
server:

```sh
DOMAIN=garden.example.com   # your address
mkdir -p ~/kvist && cd ~/kvist
curl -fsSLO https://raw.githubusercontent.com/klppl/kvist/main/deploy/docker-compose.yml
curl -fsSLO https://raw.githubusercontent.com/klppl/kvist/main/deploy/kvist.toml
sed -i "s/garden.example.com/$DOMAIN/g" kvist.toml
docker compose up -d
docker compose exec kvist kvist token create   # copy the token
```

Then unzip
[kvist-plugin.zip](https://github.com/klppl/kvist/releases/latest/download/kvist-plugin.zip)
into your vault's `.obsidian/plugins/`, enable **kvist**, enter your
domain, site `garden` and the token, put a note in a `Kvist` folder and
run **kvist: Publish now**. Step by step:
**[TLDR: quick setup](https://klppl.github.io/kvist/tldr.html)**.

## Get started

**[Read the documentation →](https://klppl.github.io/kvist/)**

- [Getting started](https://klppl.github.io/kvist/getting-started.html):
  from an empty server to your first published notes, in about 30 minutes.
- [Publishing notes](https://klppl.github.io/kvist/publishing.html): what
  becomes public, and what stays private.
- [Customizing the site](https://klppl.github.io/kvist/customizing.html):
  title, home page, menu groups and colors.
- [Domains and HTTPS](https://klppl.github.io/kvist/https.html): your
  domain with HTTPS, with the ready-made Caddy setup or your own proxy,
  and several sites on one server.
- [Running the server](https://klppl.github.io/kvist/server.html):
  updates, tokens, rollbacks, removing a site and backups.
- [Roadmap](https://klppl.github.io/kvist/roadmap.html): what doesn't work
  yet, and what's planned.

The server runs from the Docker image `ghcr.io/klppl/kvist`. The Obsidian
plugin works on desktop and mobile: download
**[kvist-plugin.zip](https://github.com/klppl/kvist/releases/latest/download/kvist-plugin.zip)** and unzip it into your vault's
`.obsidian/plugins/` folder ([details](plugin/README.md)).

## Development

```sh
make build          # bin/kvist
make test           # Go tests
make plugin-test    # plugin tests
./bin/kvist dev --dir example-vault --config kvist.example.toml   # preview the demo site
```

Requires Go 1.26 or later, and Node 20+ for the plugin. Read
[CONTRIBUTING.md](CONTRIBUTING.md) before changing anything that affects
what gets published. Developer notes: [design](docs/design.md),
[sync protocol](docs/protocol.md), [content model](docs/content-model.md),
[themes](docs/themes.md), [deployment](docs/deploy.md). The user guide's
source is `docs/*.html`.

Licensed under the [Lagom License](LICENSE).
