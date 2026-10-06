# Contributing to kvist

Thanks for helping. Start with [docs/design.md](docs/design.md): it explains
the architecture and, more importantly, the leak-prevention rules every
change must keep.

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
make test                 # gofmt check is in CI: run `make fmt` first
make plugin-test          # plugin type check, unit and end-to-end tests
go run ./cmd/kvist dev --dir example-vault --config kvist.example.toml
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

## Commits and pull requests

Small, focused commits with a message that says why. Run the tests before
opening a pull request; CI runs the same checks.
