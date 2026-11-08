# Conventions

## 1. Module and dependencies

- Module path is `gonetsurf` (no host prefix).
- Go 1.27 or newer.
- Runtime dependencies are limited to the standard library and `golang.org/x/*`.
- In-tree forks are allowed and must record their upstream revision in a `FORK.md` next
  to the fork.
- `cmd/netsurfctl deps-check` (Phase 0) fails the build if a banned module appears in
  `go list -deps`.

## 2. Package and file layout

- Category packages use plural names: `parsers`, `protocols`, `caches`, `storages`,
  `layouting`, `windowing`, `rendering`, `brokers`, `engines`, `adapters`.
- The DOM package is `dom` (singular), per explicit decision.
- Each package has a `doc.go` describing its scope in a short paragraph.
- Subpackages are narrow and named for the concept they own, not for the layer:
  `parsers/css/tokenizer`, `parsers/css/selectors`, `protocols/http1`.
- Data tables that are generated live in `*_gen.go` and are committed; the generator is
  `go:generate`-invoked and its inputs are committed and readable.

## 3. Naming

- No single-letter identifiers. Names must be descriptive: `request`, `response`,
  `selector`, `element`, `declaration`.
- Exported identifiers are documented.
- Acronyms follow Go style: `URL`, `HTTP`, `CSS`, `DOM`, but package names are lowercase.
- Booleans read as predicates: `isConnected`, `hasParent`, `canCache`.
- Avoid stutter: `http1.Client`, not `http1.HTTPClient`.

## 4. Comments

- Comments explain *why*, not *what*. Do not narrate self-evident code.
- Every exported type/function has a doc comment starting with its name.
- Normative spec references use the form `// See RFC 7230 §3.3.3.` or
  `// See CSS Syntax 3 §4.3.1.`
- No commented-out code.

## 5. Errors

- Wrap with `%w`; use sentinel errors for expected parse recoveries.
- Never panic across a package boundary. Parsers recover internally and report.
- Security-relevant rejections return typed errors that callers can test with `errors.Is`.
- Do not include untrusted content verbatim in error messages that reach logs.

## 6. Testing

Every task must define its verification before it can be marked `Verified`.

| Test kind | Requirement |
|---|---|
| Unit | Table-driven Go tests next to the code. |
| End-to-end | A test that exercises the real pipeline (fetch → parse → layout → paint or equivalent). |
| Conformance | The governing spec's test suite or vector corpus, wired into CI, with the result recorded in `task-tracking.md`. |
| Fuzz | Native Go fuzz targets for all parsers and the cache path mapper. |
| Performance | Benchmarks for parser, style, layout, paint, and network hot paths. |

- Test files are `*_test.go` in the same package (or `_test` package when public API
  testing is intended).
- Golden files live under `testdata/` and are updated only with review.
- A task with `Spec=Compliant` must cite the spec and the suite that proves it.

## 7. Task tracking discipline

- Every change references a task ID in the commit message, e.g.
  `P4-CSS-001: implement CSS tokenizer escapes`.
- Status and verification columns in `docs/task-tracking.md` are updated in the same
  change that moves the task forward.
- A task is not `Verified` until `UT=Y`, `E2E=Y`, `Spec=Compliant`, and `Bugs=-`.
- Bugs are recorded against the task ID and resolved before verification.

## 8. Embedded assets

- UA stylesheet, default error pages, and other text assets are embedded with
  `//go:embed` and kept in readable source files.
- Binary assets are avoided unless required; prefer generated data tables.

## 9. Build tags

- Platform backends use build constraints, e.g. `//go:build linux && x11`.
- The JavaScript engine is activated at runtime, not by build tag, so a single binary
  supports both modes.

## 10. Formatting and linting

- `gofmt`/`goimports` are mandatory.
- `go vet ./...` must pass.
- A linter that enforces the no-single-letter and dep-allowlist rules is part of Phase 0
  where feasible; otherwise review enforces it.

## 11. Decisions

- **D-CONV-001:** `gonetsurf` is the module path.
- **D-CONV-002:** Category packages are plural; the DOM package is `dom`.
- **D-CONV-003:** Descriptive names only; no single-letter identifiers.
- **D-CONV-004:** Commit messages reference task IDs.
