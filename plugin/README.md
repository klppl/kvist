# kvist for Obsidian

Publishes the public notes of your vault to your own
[kvist](https://github.com/klppl/kvist) server, which builds and serves them
as a website. Works on desktop and mobile.

## Install (until it is in the community plugin list)

Download `kvist-plugin-<version>.zip` from the
[releases](https://github.com/klppl/kvist/releases) and unzip it into
`<vault>/.obsidian/plugins/`, so the files end up in
`<vault>/.obsidian/plugins/kvist/`. Then enable **kvist** under
*Settings → Community plugins*.

Or build it yourself:

```sh
cd plugin
npm install
npm run build
```

and copy `main.js`, `manifest.json` and `styles.css` to
`<vault>/.obsidian/plugins/kvist/`.

Releases are made by the *Plugin release* workflow: pushing a tag like
`v0.2.0` builds the plugin and attaches the zip and the three loose files
to that tag's release. It can also be run by hand from the Actions tab.
Bump `version` in `manifest.json` (and `package.json`) before tagging.

## Set up

1. On the server, create a token for this device:
   `kvist token create --site garden --name "My phone"`.
2. In the plugin settings, enter the **server URL** (https) and the **site**.
3. Under *This device*, paste the **token**, then press **Test**.
4. Run **Open site settings note** to create `_site.md`, where you set the
   site's title, home page, menu and colors.

The server URL and site are stored in the plugin's `data.json`, so they sync
with your vault. The token, the device name and the auto-publish switch are
stored **on this device only**, so syncing tools such as LiveSync don't copy
them. Use one token per device, so you can revoke one without the others.

## Use

- **Publish now** (command palette, or the button in the report).
- **Toggle publish for current note**, also in the file menu: writes
  `publish: true` or `publish: false` to the note's frontmatter. If a rule
  wins anyway (an excluded folder, a `#private` tag), it tells you.
- **Open published page** opens the note on your site.
- **Open site settings note** opens `_site.md`, the note with the site's
  title, home page, menu and theme settings, and creates it from a template
  the first time. It's never published as a page.
- **Open publish report**, or click the status bar: links and embeds to
  notes that stay private, attachments in excluded folders, and the server's
  warnings, each with a link to the note.
- **Publish automatically** publishes after changes settle (30 s by
  default). Turn it on for **one** device: a device that hasn't finished
  syncing could otherwise publish older notes. It never overrides the
  server's "newer content" check; it stops and the status bar shows ◆. Run
  **Publish now** to review and confirm.

## What gets published

The server decides the rules (see the main README). The plugin fetches them
and applies them before uploading anything; the server checks every note
again. Only published notes, the attachments they reference, a hints file
that names published notes only, the settings note `_site.md` and
`.kvist/site.toml` (if present) leave your device.

## Development

```sh
npm test          # unit tests, plus an end-to-end run against a real server (needs Go)
npm run typecheck
npm run dev       # rebuild main.js on change
```

The publish rules and URL rules are checked against the same fixtures as
the Go server (`testdata/parity/`).
