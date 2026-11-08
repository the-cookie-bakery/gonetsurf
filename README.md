# gonetsurf

A privacy-preserving, pure-Go web browser engine.

`gonetsurf` fetches, parses, styles, lays out, and paints web pages. The default
build is pure Go with no cgo and no third-party runtime dependencies. It ships with a native X11 shell, headless
rendering tools, a network/storage broker, and a specification-driven test suite.

The engine is under active development. It is already usable for static pages; JavaScript,
the full Web API surface, and Wayland support are later phases.

## Status

| Area | State |
|---|---|
| Phases 0–6 | Complete and verified |
| Phase 7 — paint/fonts/images/color | 12 verified, 1 partial (OpenType feature/variation shaping) |
| Phase 8 — window and UI shell | Complete and verified |
| Phase 9 — brokers and sandboxing | Complete and verified |
| Phase 10 — JavaScript/Web APIs/Wayland | Not started |

Current ledger totals: **97 Verified, 2 Partial, 18 Planned, 0 Blocked**.

Known gaps:

- JavaScript and Web APIs are not implemented.
- Wayland is not implemented; the native backend is X11.
- `::before`/`::after` generated content supports strings and `attr()`; counters,
  `content: url()`, and other pseudo-elements are not implemented.
- WOFF2 requires a Brotli decoder and is currently rejected.
- Full OpenType GSUB/GPOS shaping is deferred to `P10-TEXT-001`.
- The WPT reftest corpus is not vendored; local goldens and the reftest harness are used.
- TLS GREASE/ECH needs the planned ClientHello fork.

See [`docs/handover.md`](docs/handover.md) for the detailed snapshot and
[`docs/task-tracking.md`](docs/task-tracking.md) for the task-by-task ledger.

## Highlights

- **URL and encoding:** WHATWG URL parser/serializer, origin/site computation, IDNA,
  encoding sniffing, and HTML microsyntaxes.
- **Networking:** HTTP/1.1, TLS 1.3, DNS/DoH/DoT, SOCKS5, WebSocket, redirects,
  conditional requests, ranges, and MIME handling.
- **Caching and cookies:** RFC 7234 semantics, aggressive archive caching, invertible
  disk mapping, partitioned cookies, and tracking-session resets.
- **HTML and DOM:** `golang.org/x/net/html` integration, node budgets, quirks modes,
  template contents, shadow-DOM scaffolding, and selector queries.
- **CSS:** Syntax 3 tokenizer/parser, Selectors 4, Values 4 math and units, Color 4,
  CSSOM, cascade/layers/custom properties, conditions, media queries, animations,
  nesting, and a generated property registry.
- **Style and layout:** indexed selector matching, UA stylesheet, computed values,
  incremental restyle, generated `::before`/`::after` boxes, block/inline/table/
  flex/grid formatting, floats, positioning, writing modes, bidi, text shaping,
  overflow, and incremental layout.
- **Rendering:** anti-aliased rasterizer, borders/backgrounds/radii/shadows/gradients,
  rounded content and overflow clipping, PNG/JPEG/GIF/WebP images, SVG subset,
  fonts and `@font-face`, Color 4 conversion, stacking/opacity/2D+3D transforms,
  compositor damage tracking, and PNG output.
- **Browser shell:** tabs, history, session restore, opener/child contexts, same-document
  navigation, inline iframes, page and nested scrolling, autocomplete/search, tab
  reordering, rasterized chrome, and a broker-backed privacy dashboard.
- **Isolation:** capability-scoped IPC, supervised processes, Linux sandbox primitives,
  a network broker, and a partitioned storage broker.

External stylesheets and nested `@import` rules are loaded through `adapters`, with
resource URLs resolved relative to their stylesheet. Scroll rendering caches chrome,
font matching, colors, per-rune fallback, and blits, and skips offscreen text before
shaping.

## Requirements

- Go **1.27+**
- Linux for the interactive X11 shell
- An X11 server and `DISPLAY` for `cmd/netsurf` and `netsurfctl show`

The default software renderer requires no cgo. The optional Vulkan renderer is
built with `-tags vulkan`, requires cgo, the system Vulkan loader, and shader
tooling (`glslc`, `spirv-val`). See [`rendering-refactor.md`](rendering-refactor.md).

## Quick start

```sh
git clone https://github.com/cookiengineer/gonetsurf.git
cd gonetsurf

go build ./...
go run ./cmd/netsurf https://example.com/
```

Run the full verification suite:

```sh
make compliance-tests
```

This runs the build, `go vet`, formatting check, dependency policy check, and all
unit/conformance/E2E tests.

## Tools

`cmd/netsurfctl` provides diagnostics and headless rendering:

```sh
go run ./cmd/netsurfctl deps-check
go run ./cmd/netsurfctl urlparse 'https://example.com:8443/a/b?x=1#f'
go run ./cmd/netsurfctl fetch https://example.com/
go run ./cmd/netsurfctl layout page.html 1280 800
go run ./cmd/netsurfctl render page.html page.png 1280 800
go run ./cmd/netsurfctl render https://cookie.engineer/index.html page.png 1280 800
go run ./cmd/netsurfctl reftest test.html reference.html 800 600
go run ./cmd/netsurfctl show page.html 1280 800   # requires DISPLAY
```

Useful development commands:

```sh
make conformance                 # specification conformance suites
make fuzz FUZZTIME=30s           # short fuzz runs across parser targets
make bench                       # parser/layout/paint/font/X11 benchmarks
make fmt                         # gofmt the tree
make corpus-refresh              # refresh vendored test corpora
```

## Architecture

```text
shells/ + windowing/x11
        │  input, chrome, tabs
        ▼
browsers/ ── urls ── parsers/html ── dom
                              │
                              ▼
                         parsers/css ── styles ── layouts ── rendering
                                                              │
                                                              ▼
                                                           surfaces

brokers/network ◄── protocols/{http1,tls,dns,socks,websockets}
brokers/storage ◄── storages/cookies, caches/*
        ▲
        │ capability-scoped IPC + shared memory
runtime/ + runtime/sandbox
```

Main packages:

| Path | Responsibility |
|---|---|
| `urls/` | WHATWG URL parsing, serialization, origins, sites, internal schemes |
| `parsers/html/` | Encoding sniffing, HTML parsing, DOM conversion, fragments |
| `parsers/css/` | Tokenizer, parser, grammar, selectors, values, colors, CSSOM |
| `styles/` | UA sheet, selector matching, cascade, computed values, restyle |
| `layouts/` | Box tree, formatting contexts, sizing, positioning, text, overflow |
| `rendering/` | Rasterizer, paint, fonts, images, color, SVG, compositor, `Page` |
| `windowing/` | Display/window abstraction, headless and X11 backends |
| `browsers/`, `shells/` | Tabs/navigation/session model and interactive browser UI |
| `protocols/` | HTTP/1.1, TLS, DNS, SOCKS5, WebSocket, MIME, internal schemes |
| `caches/`, `storages/` | HTTP/disk/memory caches, cookies and partition storage |
| `brokers/` | Network and storage broker processes |
| `ipc/` | Framed messaging, capabilities, shared memory, fd passing |
| `runtime/` | Process supervision and Linux sandbox primitives |
| `adapters/` | Stylesheet loading and cross-subsystem bridges |
| `cmd/` | `netsurf`, `netsurfctl`, corpus/property generators |

See [`docs/architecture.md`](docs/architecture.md) for the full design and process model.

## Verification

The test suite covers unit, conformance, fuzz, benchmark, golden-pixel, reftest, and
integration paths. Current highlights:

- WHATWG URL corpus: **896/896**
- CSS tokenizer: **223/223**
- Property registry: **353** generated properties
- CSS value corpus: **2,520 passes, 0 failures, 260 documented skips**
- An+B: **128/128**
- IDNA: **2,669 cases**
- HTML tree construction: **1,578 cases**
- Layout: 35-case geometry harness plus paint integration tests
- Real-site checks against headless Chromium on `cookie.engineer`

A representative long-page shell scroll frame is benchmarked at roughly the 60 Hz
budget.

## Dependency policy

- Module path: `gonetsurf`
- Go: 1.27+
- Runtime dependencies: standard library and `golang.org/x/*` only
- In-tree forks are allowed and must record their upstream revision in `FORK.md`
- `go run ./cmd/netsurfctl deps-check` enforces the policy

Current dependencies are `golang.org/x/image`, `golang.org/x/net`, `golang.org/x/sys`,
and `golang.org/x/text`.

## Privacy and security

- Network and storage access are capability-scoped through broker processes.
- Cookies and caches are partitioned by top-level site.
- Tracking sessions and CNAME-cloaked trackers are handled as a dependency graph.
- The shell applies a restrictive Linux sandbox baseline and an INET-socket guard.
- The renderer is designed to run as untrusted content without ambient network or
  filesystem access.

Read [`docs/privacy.md`](docs/privacy.md) and [`docs/security.md`](docs/security.md)
before changing transport, storage, or sandbox behavior.

## Documentation

| Document | Purpose |
|---|---|
| [`docs/handover.md`](docs/handover.md) | Current state, limitations, and next steps |
| [`docs/implementation-plan.md`](docs/implementation-plan.md) | Phases, tasks, and acceptance criteria |
| [`docs/task-tracking.md`](docs/task-tracking.md) | Master status ledger |
| [`docs/architecture.md`](docs/architecture.md) | Process model and subsystem boundaries |
| [`docs/cache.md`](docs/cache.md) | Cache policy and URL↔path mapping |
| [`docs/privacy.md`](docs/privacy.md) | Tracking sessions, partitioning, and farbling |
| [`docs/security.md`](docs/security.md) | Threat model and sandboxing |
| [`docs/conventions.md`](docs/conventions.md) | Code, test, and naming conventions |
| [`docs/specifications.md`](docs/specifications.md) | Pinned specifications and provenance |
| [`AGENTS.md`](AGENTS.md) | Build, test, and contribution commands |

Normative specifications live in [`specifications/`](specifications/) and are pinned
snapshots. Refresh them with `sh specifications/update.sh`.

## Contributing

- Follow [`docs/conventions.md`](docs/conventions.md).
- Reference a task ID from [`docs/task-tracking.md`](docs/task-tracking.md) in commit
  messages, for example: `P8-SHELL-002: fix nested scroll painting`.
- Update the task ledger in the same change that moves a task forward.
- Run `make compliance-tests` before opening a pull request.
- Add fuzz targets and benchmarks when introducing a new parser or hot path.
- Do not add third-party runtime dependencies.
- Keep the default build cgo-free; optional backends use build tags.
