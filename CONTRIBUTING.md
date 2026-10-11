# Contributing to kvist

Thanks for helping. Start with [docs/design.md](docs/design.md): it explains
the architecture and, more importantly, the leak-prevention rules every
change must keep. [AGENTS.md](AGENTS.md) is the short version: a map of the
code, the invariants, and which files to update together for common changes.

## Layout

```
cmd/kvist/          CLI (serve, push, build, dev, token, site, rollback, gc)
internal/           Go packages (see design.md §10)
themes/garden/      the built-in theme (embedded in the binary)
plugin/             the Obsidian plugin (TypeScript)
testdata/leaks/     leak-prevention fixtures
testdata/parity/    fixtures shared by the Go and plugin tests
docs/               the user guide (*.html, GitHub Pages) and the developer
                    notes: design, protocol, content model, themes, deployment
```

## Develop

```sh
make check                # everything CI runs (gofmt check, vet, tests, plugin)
make fmt                  # gofmt; CI fails on unformatted Go
make test                 # Go vet and tests
make plugin-test          # plugin type check, unit and end-to-end tests
make dev                  # preview example-vault with live reload on :1313
```

You don't need Obsidian to work on the server: `kvist push --dir` is a
complete client, and `kvist dev` previews a folder with live reload.

## Rules for changes

- **Nothing private may reach the output.** Anything new that reads notes
  (a page type, a generated file, a template function, a model field) must
  work from the published model only. Add the vector to
  `testdata/leaks/basic` with a `LEAKMARK` string and make sure the leak
  tests in `internal/model` and `internal/build` still pass.
- **Both gates decide the same way.** If you change the publish rules,
  change `internal/publish` and `plugin/src/rules.ts` together and extend
  `testdata/parity/rules.json`.
- **Protocol and content model are contracts.** Additive changes are fine;
  anything else needs a version bump and an entry in `docs/protocol.md` or
  `docs/content-model.md`.
- **Builds are deterministic.** No wall-clock time, map iteration order or
  randomness in the output.
- Keep dependencies few. The server uses goldmark, chroma, yaml.v3,
  BurntSushi/toml, x/text and x/image (for social images); the plugin uses only the Obsidian API.

## Translations

There are two sets of words, translated separately:

- **The site** (the theme's menus, dates and search) follows the site's
  `language`. Its words are in `themes/garden/i18n/<lang>.toml`, with
  `en.toml` as the source; see [docs/themes.md](docs/themes.md).
- **The plugin** follows Obsidian's language. Its words are in
  `plugin/src/i18n/<lang>.ts`, with `en.ts` as the source; see the
  [plugin README](plugin/README.md#language).

Add a new word to English first, then to every other language; the tests
in `internal/render` and `plugin/test/i18n.test.ts` fail until each
language has it. The CLI and the server's log and error messages stay in
English.

## Commits and pull requests

Small, focused commits with a message that says why, with a subject like
`Area: what changed`. Run `make check` before opening a pull request; CI
runs the same checks.
