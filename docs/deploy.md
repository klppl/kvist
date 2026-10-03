# Deploying kvist

This guide sets up kvist on a small Linux server (for example a Hetzner
CX22) with HTTPS, optionally behind Cloudflare. kvist is one static binary.
It needs very little memory and keeps everything in `data_dir`.

## 1. Server

Any Linux VM with a public IP. Point DNS for your site (for example
`garden.example.com`) at it. Open ports 80 and 443; kvist itself listens
on localhost or a private Docker network only.

## 2a. Run with Docker (recommended)

```sh
git clone https://github.com/klppl/kvist && cd kvist/deploy
$EDITOR kvist.toml Caddyfile        # your domain, title and publish rules
docker compose up -d
docker compose exec kvist kvist token create --site garden --name laptop
```

Caddy gets a certificate from Let's Encrypt and proxies everything,
including `/api/`, to kvist, which builds the site and serves it
(`serve = true`). The data lives in the `kvist-data` volume.

## 2b. Run with systemd

```sh
make build                           # bin/kvist, a static binary
sudo useradd --system --home /var/lib/kvist --create-home kvist
sudo install -m 0755 bin/kvist /usr/local/bin/kvist
sudo install -D -m 0640 -g kvist kvist.example.toml /etc/kvist/kvist.toml
sudo $EDITOR /etc/kvist/kvist.toml   # data_dir = "/var/lib/kvist", listen = "127.0.0.1:8080"
sudo cp deploy/kvist.service /etc/systemd/system/
sudo systemctl enable --now kvist
sudo -u kvist KVIST_CONFIG=/etc/kvist/kvist.toml kvist token create --site garden --name laptop
```

Put Caddy (see `deploy/Caddyfile`) or nginx in front for HTTPS. With nginx,
you can serve the built files directly and proxy only the API:

```nginx
server {
    server_name garden.example.com;
    root /var/lib/kvist/sites/garden/public;   # only ever serve public/
    error_page 404 /404.html;
    location /api/ { proxy_pass http://127.0.0.1:8080; client_max_body_size 60m; proxy_read_timeout 90s; }
    location /_assets/ { add_header Cache-Control "public, max-age=31536000, immutable"; }
    location /_kvist/  { add_header Cache-Control "public, max-age=31536000, immutable"; }
    location / { try_files $uri $uri/index.html =404; add_header Cache-Control "public, max-age=60"; }
}
```

Never point a web server at `sites/<id>/` or `data_dir` itself: older builds
and stored revisions live there. Only `public/` is meant to be served.

## 3. Connect Obsidian

Install the plugin (`plugin/README.md`), then enter the server URL
(`https://garden.example.com`), the site id and the token. Press **Test**,
then **Publish now**. Create one token per device; `kvist token list` and
`kvist token revoke ID` manage them.

## 4. Cloudflare (optional)

With the DNS record proxied (orange cloud):

- **SSL/TLS mode:** Full (strict). Caddy already has a valid certificate.
- **Caching:** kvist sends `Cache-Control: public, max-age=60` for pages and
  `max-age=31536000, immutable` for `/_assets/` and `/_kvist/` (their URLs
  contain a content hash). Leave Cloudflare's cache settings on "respect
  existing headers". Don't add a "cache everything" rule with a long edge
  TTL, or unpublished pages stay visible until it expires.
- **Bypass the API:** add a Cache Rule for `URI Path starts with /api/` →
  *Bypass cache*. kvist sends `no-store` there anyway.
- **Upload size:** Cloudflare's free plan limits request bodies to 100 MB.
  kvist's default `max_file_size` is 50 MiB.
- **Purge on publish:** create an API token with only *Zone → Cache Purge*
  for the zone, and add to the site config:

  ```toml
  [site.cloudflare]
  zone_id       = "…"             # Zone ID from the domain's overview page
  api_token_env = "CF_API_TOKEN"  # or api_token_file = "/etc/kvist/cf-token"
  ```

  After every successful build kvist purges the zone, so an unpublished
  page disappears from the edge right away. If the purge fails, the build
  still succeeds and the warning appears in the plugin.

## Operations

- **Backups:** back up `data_dir` (or the Docker volume). `tokens.json`
  holds only token hashes. Your vault is the real source of truth: losing the
  server only means pushing again.
- **Undo a publish:** `kvist rollback garden r000041` makes an earlier
  revision current again (the last 10 are kept). Then fix the vault, or the
  next push from a device replaces it again.
- **Retention:** unpublished content leaves the website at once but stays in
  older revisions and builds on the server's disk until they drop out of
  `[site.retention]` (10 revisions, 3 builds by default). Set both to 1 to
  keep the minimum.
- **Disk:** builds share unchanged files through hard links, so retained
  builds cost little extra space.
- **Upgrades:** replace the binary (or `docker compose build && docker
  compose up -d`). The plugin and server check protocol versions and tell
  you if one side needs an update.
- **Logs:** `journalctl -u kvist` or `docker compose logs kvist`. Every
  commit, build and failure is logged.
