# Architecture

> **Status:** this document describes the target architecture. For what is actually
> implemented today, what is partial, and where to start, see
> [`handover.md`](./handover.md). As of 2026-10-28 the implemented packages include the
> URL, encoding, HTML-parsing/`dom`, HTTP/TLS/DNS, cache, cookie, Phase 4 CSS
> parser/CSSOM layers, the Phase 5 `styles` engine, the Phase 6 `layouting` engine,
> the Phase 7 `rendering` pipeline (paint, fonts, images, color, compositor), the
> Phase 8 `windowing`, `browsers`, and `shells` packages with the X11 backend,
> the Phase 9 `ipc`, `runtime`, `runtime/sandbox`, and `brokers` packages, and the
> `adapters` stylesheet-loading bridge. Web APIs and JavaScript remain design
> targets.

## 1. Scope

`gonetsurf` is a privacy-preserving, pure-Go web browser engine. The first delivery target
is a **native windowed browser** on Linux (X11 first, then Wayland) that:

- fetches over **TLS 1.3 + HTTP/1.1**,
- parses HTML with `golang.org/x/net/html`,
- parses CSS with a fully tokenized, lexed, specification-compliant parser and CSSOM,
- performs **modern full layout** (block/inline/table/flex/grid/positioning),
- paints to a framebuffer, and
- defines a **pluggable, activatable, farble-able** JavaScript engine interface with a
  vendored `sobek` fork as the eventual backend.

JavaScript execution and the full Web API surface are a later phase, but their interfaces
and privacy hooks are designed now.

## 2. Process model

```
                         ┌───────────────────────────────────────────────┐
   window events  ◄─────►│ shells/            browser UI + tab model      │
                         │ windowing/{x11,wayland,headless}               │
                         └───────────────┬───────────────────────────────┘
                                         │ ipc/  (Unix socket + shm surfaces)
                         ┌───────────────▼───────────────────────────────┐
                         │ engines/browsing   one process per site        │
                         │   dom → styles → layouting → rendering           │
                         │   javascripts (sobek) + webapis (farbled)      │
                         │   sandboxed: no sockets, no ambient filesystem │
                         └───────────────┬───────────────────────────────┘
                                         │ ipc/ (capability-scoped requests)
                         ┌───────────────▼───────────────────────────────┐
                         │ brokers/network    single trusted process      │
                         │   protocols/{http1,tls,dns,socks,websockets}   │
                         │   caches/http → ~/NetSurf/cache/               │
                         │   storages/{cookies,origins}                   │
                         └───────────────────────────────────────────────┘
```

### 2.1 Process granularity

| Process | Count | Owns | Trust |
|---|---|---|---|
| Shell / UI | 1 | Windows, tab model, chrome, input | Trusted application code; no web content. |
| Renderer | 1 per site instance | DOM, CSS, layout, paint, JS | **Untrusted** web content. |
| Network broker | 1 | Sockets, DNS, TLS, cookies, cache | Trusted; validates all input. |

A **site instance** is initially the registrable domain (eTLD+1, resolved with
`golang.org/x/net/publicsuffix`). Cross-site navigation creates or reuses a renderer for
the destination site. A strict per-origin mode is a configuration option. The broker is a
separate, persistent process so that renderer restarts do not lose connections or cache
state.

### 2.2 Why this shape

- Renderer compromise (malicious HTML/CSS/JS, parser or rasterizer bug) must not yield
  sockets, the cookie jar, or arbitrary filesystem access.
- The broker is the single place where network policy (HTTPS-only, DoH, partitioned
  cookies/cache, header normalization) is enforced, so it cannot be bypassed by content.
- One process per site limits cross-site memory disclosure and lets a crashed tab be
  isolated and restarted.

## 3. Subsystem map

| Directory | Responsibility |
|---|---|
| `cmd/netsurf` | Entry point; re-exec modes `--renderer` and `--broker`. |
| `cmd/netsurfctl` | Cache export/import/verify, conformance runner, diagnostics. |
| `urls` | WHATWG URL parser/serializer and origin. Single source of truth for security keys. |
| `parsers/html` | `x/net/html` integration, encoding sniffing, HTML microsyntaxes. |
| `dom` | Our DOM tree (`Node`, `Element`, `Document`, shadow DOM, template contents). |
| `parsers/css` | Tokenizer, parser, grammar engine, selectors, values, CSSOM, cascade, media, animations. |
| `styles` | Style resolution, inheritance, computed values, UA stylesheet, presentational hints. |
| `layouting` | Backend-neutral box tree, formatting contexts, software/Vulkan layout backends. |
| `rendering` | Paint frontend, page orchestration, image providers, and renderer selection. |
| `rendering/geometry` | Backend-neutral value types, path/coverage math, transforms. |
| `rendering/interfaces` | Surface, Canvas, Renderer, Backend, Presenter, and scene contracts. |
| `rendering/scene` | Display-list recorder used by non-software backends. |
| `rendering/software` | CPU surface, rasterizer, compositor, and SVG rasterizer. |
| `rendering/vulkan` | Optional cgo/Vulkan backend (scaffolded). |
| `rendering/utils/css` | Shared CSS value parsing helpers. |
| `windowing` | Display abstraction plus X11, Wayland, and headless backends. |
| `shells` | Browser chrome, tabs, omnibox, history, privacy dashboard. |
| `browsers` | Tab/session/navigation/history model. |
| `protocols` | HTTP/1.1, TLS, DNS, SOCKS, WebSocket, internal schemes. |
| `caches` | RFC 7234 HTTP cache, disk mapping/journal, in-memory LRU. |
| `storages` | Cookies, origin storage, settings, history, bookmarks. |
| `brokers` | Network and storage broker processes. |
| `ipc` | Framing, capability tokens, shared-memory surfaces. |
| `runtime` | Process supervision, sandboxing, resource limits. |
| `farbling` | Deterministic per-origin spoofing for Web APIs. |
| `webapis` | Window/document/navigator/canvas/etc. bindings. |
| `engines/javascripts` | JS engine interface and registry; `sobek` binding. |
| `javascripts/sobek` | Vendored, dependency-pruned JS engine fork. |
| `adapters` | Small bridges between subsystems. |

## 4. Data flow of a page load

1. Shell resolves the URL with `urls` and asks the site instance for a renderer.
2. Renderer requests the document from the broker over `ipc` with a capability token.
3. Broker checks the partition, cookies, and `caches/http`; on miss, `protocols/http1`
   (over `protocols/tls`, via `protocols/dns`) fetches it and stores it.
4. Renderer decodes bytes (`parsers/html` encoding sniffing), parses with `x/net/html`,
   and builds the `dom` tree.
5. `parsers/css` parses styles; `styles` cascades and computes values.
6. `layouting` builds the box tree and lays out; `rendering` paints into a surface.
7. The surface is transferred to the shell over shared memory and composited to the window.
8. JavaScript (later phase) runs through `engines/javascripts` with `webapis` bindings that
   consult `farbling`.

Subresources (`img`, `css`, `font`, `script`) follow the same renderer→broker path and are
deduplicated by the broker's single-flight cache.

## 5. Core interfaces (design targets)

### 5.1 JavaScript engine (pluggable / activatable / farble-able)

```go
// engines/javascripts
type Engine interface {
    Name() string
    NewRealm(ctx context.Context, host RealmHost) (Realm, error)
}

type Realm interface {
    SetDocument(document dom.Document)
    RunScript(sourceName, source string) error
    DispatchEvent(event webapis.Event) error
    Close() error
}

type RealmHost interface {
    Cache() caches.HTTP
    Storage(origin urls.Origin) storages.Origin
    Farbling() *farbling.Profile
    Fetch(ctx context.Context, request protocols.Request) (protocols.Response, error)
}
```

- **Pluggable:** engines register in a registry; the shell selects one.
- **Activatable:** `javascript.Enabled=false` makes `<script>` inert and renders `<noscript>`.
- **Farble-able:** every `webapis` binding receives the `farbling.Profile`, so observable
  values are spoofed consistently per origin.

### 5.2 Display

```go
// windowing
type Display interface {
    Open() (Window, error)
    EventLoop(ctx context.Context) error
    Close() error
}

type Window interface {
    Surface() rendering.Surface
    Present(damage image.Rectangle) error
    SetTitle(title string)
    Size() (width, height int)
    Scale() float64
}
```

Implementations: `windowing/headless`, `windowing/x11`, `windowing/wayland`.

### 5.3 Broker transport

```go
// ipc
type Transport interface {
    Send(ctx context.Context, message Message) error
    Receive(ctx context.Context) (Message, error)
    ShareMemory(size int) (SharedBuffer, error)
}
```

### 5.4 Text shaping (deferred complexity boundary)

```go
// rendering/fonts
type TextShaper interface {
    Shape(run TextRun) []Glyph
}
```

Phase 1 uses `SimpleShaper` (advance widths plus kerning). A full OpenType GSUB/GPOS
shaper implements the same interface later.

## 6. Concurrency model

- `context.Context` on all entry points; a navigation cancels in-flight work.
- Each renderer runs a stage pipeline (parse → style → layout → paint) with bounded worker
  pools and a per-stage work queue.
- Parallelizable work: CSS selector matching, style resolution by tree depth, image
  decoding, text measurement, tile painting, broker fetches.
- Inherently sequential work: box-tree dependent layout passes; handled with dirty flags.
- The broker uses single-flight deduplication and bounded per-host connection pools.
- Backpressure and cancellation travel over `ipc`; a renderer that stops draining is
  killed by the supervisor.

## 7. State, storage, and partitioning

| State | Location | Default lifetime |
|---|---|---|
| HTTP cache | `~/NetSurf/cache/` | Persistent archive (see `cache.md`). |
| Cookie jar | `~/NetSurf/cookies/` | Persistent, tracking-session scoped (see `privacy.md`). |
| Origin storage | `~/NetSurf/storage/` | Ephemeral per tab unless persisted. |
| Settings | `~/NetSurf/config/` | Persistent. |
| History/bookmarks | `~/NetSurf/` | Persistent, local only. |

All web-derived state is **partitioned by top-level site** so that a third-party origin
cannot correlate visits across different first-party sites.

## 8. Decisions

- **D-ARCH-001:** OS process per site instance, broker-mediated network and storage.
- **D-ARCH-002:** `x/net/html` is mandatory and is only wrapped, not re-implemented, unless
  scripting/encoding hooks force driving `html.Tokenizer` directly.
- **D-ARCH-003:** The WHATWG URL parser in `urls` is the only parser used for security
  decisions; `net/url` is forbidden on security and cache-key paths.
- **D-ARCH-004:** DOM package is `dom` (singular). Plural convention applies to category
  packages (`parsers`, `protocols`, `caches`, `storages`, `layouting`, `windowing`).
- **D-ARCH-005:** Module path is `gonetsurf` with no host prefix.
- **D-ARCH-006:** Package skeletons are created when their phase begins, not as a single
  up-front tree of empty stubs. Every package carries a package doc comment; `cmd/*`
  packages use `// Command <name>` and libraries use `// Package <name>`.
