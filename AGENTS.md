# AGENTS.md

Build and verification commands for the `gonetsurf` project.

## Commands

```sh
make compliance-tests                   # build, vet, fmt-check, deps-check, all tests
make conformance                        # specification conformance suites only
make fuzz FUZZTIME=30s                  # short fuzz runs for the parser packages
make bench                              # parser benchmarks
make corpus-refresh                     # re-download and regenerate vendored corpora
make shaders                            # compile Vulkan GLSL to gitignored SPIR-V
make vulkan-build                       # optional cgo + -tags vulkan build
make vulkan-test                        # optional Vulkan-tagged tests

go build ./...                          # compile everything
go vet ./...                            # static checks
gofmt -l cmd urls parsers protocols caches storages internal dom styles layouts rendering windowing browsers ipc runtime brokers shells adapters   # formatting check
go test ./... -count=1                  # unit, conformance, and E2E tests
go test ./urls/ -run TestURLParserConformance -v
go run ./cmd/netsurfctl deps-check      # enforce the no-third-party policy
go run ./cmd/netsurfctl urlparse <url>  # inspect a parsed URL
go run ./cmd/netsurfctl layout page.html 800 600  # parse, style, and lay out a page
go run ./cmd/netsurfctl render page.html page.png 800 600  # parse, style, lay out, and paint to PNG
go run ./cmd/netsurfctl show page.html 800 600  # paint into an X11 window (requires DISPLAY)
go run ./cmd/netsurf https://example.com/      # run the interactive browser shell (requires DISPLAY)
```

Fuzzing (run briefly in CI, longer locally):

```sh
make fuzz FUZZTIME=30s                 # every registered target in one pass
go test ./urls/ -run='^$' -fuzz='^FuzzParse$' -fuzztime=30s   # a single target
```

Registered targets cover every parser package, the style shorthand expander,
the rasterizer, the image decoder, the color converter, IPC framing, and the
cache path mapper. Add a target when a new parser is
introduced.

## Rules

- Module path is `gonetsurf` (no host prefix). Go 1.27+.
- Runtime dependencies are limited to the standard library and `golang.org/x/*`.
  In-tree forks record their upstream revision in a `FORK.md`.
- Category packages are plural (`parsers`, `protocols`, `caches`, `storages`,
  `layouts`, `windowing`); the DOM package is `dom`.
- No single-letter identifiers. Descriptive names only.
- Commit messages reference a task ID from `docs/task-tracking.md`, e.g.
  `P1-ENC-001: implement encoding sniffing`.
- Update the status columns in `docs/task-tracking.md` in the same change that
  moves a task forward. A task is `Verified` only when `UT=Y`, `E2E=Y`,
  `Spec=Compliant`, and `Bugs=-`.

## Documentation

- `docs/implementation-plan.md` — phases, tasks, acceptance criteria.
- `docs/task-tracking.md` — the master status ledger.
- `docs/architecture.md`, `docs/cache.md`, `docs/privacy.md`, `docs/security.md`.
- `specifications/` — pinned normative documents (refresh with
  `sh specifications/update.sh`).
