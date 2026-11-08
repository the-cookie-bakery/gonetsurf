# Implementation Plan

This document defines every phase, feature, and task. The status of each task (including
its unit-test, end-to-end-test, bug, and specification-compliance state) lives in
[`task-tracking.md`](./task-tracking.md); this document is the stable specification of the
work.

> **Progress (2026-10-28):** Phases 0–3 are complete and verified, and Phase 4 (CSS) is
> complete with all 14 tasks `Verified`. The property registry contains 353 generated
> properties; the stable WPT value corpus reports 2,520 passes, 0 failures, and 260
> documented skips, and the selector parsing suite has 0 failures with 78 tentative-feature
> skips. CSSOM, cascade/variables, conditional rules, media queries, animations, and nesting
> have unit and Go-native conformance tests. Phase 5 (style resolution) is complete with
> all 6 tasks `Verified`: indexed selector matching, UA stylesheet, presentational hints,
> cascade/inheritance, custom properties, computed values, conditionals, and incremental
> restyle. Phase 6 (layout) has completed its layout-semantics pass: box-tree
> construction, min/max/aspect-ratio sizing, block/inline/table/flex/grid formatting,
> negative and collapsed-through margins, line boxes with alignment and inline-blocks,
> line-level bidi, floats with clearance, padding-box positioning and sticky offsets,
> vertical writing modes and logical properties, generic-family text metrics with pair
> kerning, scrollbar-aware overflow, and dirty-subtree reflow with clean-subtree reuse
> are implemented. All 14 Phase 6 tasks are `Verified`, including sticky paint
> integration, real font metrics/shaping, and scrollbar painting. A 35-case geometry conformance
> harness and `netsurfctl layout` cover the pipeline. Phase 7 is nearly complete: the
> `rendering` rasterizer is `Verified`, backgrounds/borders/radii/shadows,
> PNG/JPEG/GIF/WebP image decoding, Color 4 conversions, linear/radial/conic
> gradients, real font matching/shaping/glyph caching, and stacking/opacity/2D
> transforms with replaced images, @font-face, SVG, and compositor/damage
> tracking are implemented; the headless renderer, pixel comparator, and
> reftest runner are implemented (`netsurfctl render`, `netsurfctl reftest`),
> and the X11 shell renders interactive pages (`netsurf show`).
> Phase 7 has 12 tasks Verified and 1 Partial (OpenType feature/variation
> shaping, dependent on P10-TEXT-001). Phase 8 is complete: X11 input,
> clipboard, MIT-SHM, browser tabs/session and browsing-context model, page
> scrolling, link/fragment/iframe navigation, omnibox search/autocomplete, tab
> reordering, rasterized chrome, the privacy dashboard, and the interactive
> `cmd/netsurf` shell. Real-site rendering work also added linked/imported
> stylesheet loading with stylesheet-relative URL resolution, multi-keyword
> `display`, `calc()` and auto-margin box sizing, clipped-overflow containment,
> nested scroll-container hit-testing and painting, generated
> `::before`/`::after` content boxes, inline-block baselines, resize-driven
> media/layout updates, and cached scroll rendering. Phase 9 IPC
> framing/shared memory, process
> supervision, sandbox primitives, the network broker (cache/cookies + INET
> seccomp guard), and the persistent storage broker are Verified. Phase 9 is
> complete; Phase 10 and the cross-cutting backlog remain.
> See [`handover.md`](./handover.md) for the status and gaps.

## How to read a task

```
P<phase>-<AREA>-<nnn>  Title
  Depends on:  other task IDs or "none"
  Deliverable: the concrete artifact produced
  Acceptance:  what must be true to mark the task Verified
  Spec:        governing specification file(s) under specifications/
```

A task is only `Verified` when it is unit tested, end-to-end tested, has no open bugs, and
its `Spec` column is `Compliant` (or `N/A` for non-normative tooling).

---

## Phase 0 — Foundations

**Goal:** a buildable, testable repository with conventions, dependency policy, and CI
before any feature code.

### Features
- Go module `gonetsurf`, Go 1.27.
- Directory skeleton for all subsystems with `doc.go` stubs.
- Dependency allow-list enforcement in CI.
- Documentation set in `docs/`.
- Test harness scaffolding and conformance data layout.

### Tasks

- **P0-CORE-001** Module and directory skeleton.
  - Deliverable: `go.mod` (`module gonetsurf`) and package directories created as their
    phase begins, each with a package doc comment (`cmd/*` use `// Command`, libraries use
    `// Package`). Broad empty scaffolding is intentionally avoided (D-ARCH-006).
  - Acceptance: `go build ./...` and `go vet ./...` succeed, and every package carries a
    package doc comment.
  - Spec: N/A.
- **P0-CORE-002** Dependency policy check.
  - Deliverable: a `cmd/netsurfctl deps-check` subcommand plus a CI job that fails on any
    non-allowed module in `go list -deps`.
  - Acceptance: introducing a banned third-party import fails CI.
  - Spec: N/A.
- **P0-CORE-003** Verified-code ledger and `AGENTS.md`.
  - Deliverable: root `AGENTS.md` with build/test/lint commands; `task-tracking.md`
    process documented.
  - Acceptance: a contributor can follow `AGENTS.md` to build and test.
  - Spec: N/A.
- **P0-CORE-004** Panic-recovery and error conventions.
  - Deliverable: `internal` error helpers and a renderer panic barrier.
  - Acceptance: a panic in a parser does not crash the renderer process boundary.
  - Spec: N/A.
- **P0-CORE-005** Fuzzing harness scaffolding.
  - Deliverable: `go test -fuzz` targets that compile for every parser package.
  - Acceptance: `go test ./...` runs; fuzz targets registered (seeds may be empty).
  - Spec: N/A.

**Exit criteria:** `go build ./...`, `go test ./...`, `go vet ./...`, and the dependency
check all pass on an empty feature set.

---

## Phase 1 — URL, Encoding, and Microsyntax Primitives

**Goal:** the primitives every other subsystem depends on.

### Features
- WHATWG URL parser/serializer, origin, and IDNA host handling.
- HTML encoding sniffing and byte→code-point decoding.
- HTML common microsyntax parsers.

### Tasks

- **P1-URL-001** WHATWG basic URL parser.
  - Depends on: none.
  - Deliverable: `urls.Parse`, `urls.ParseReference`, state-machine implementation
    (scheme start through fragment), special-scheme backslash handling, dot-segment
    resolution, percent-encode sets.
  - Acceptance: passes the URL Standard test vectors and the WPT `url` suite subset we
    vendor.
  - Spec: URL Standard (`specifications/web/url.html`).
- **P1-URL-002** URL serializer and canonical form.
  - Deliverable: `urls.URL.Serialize`, default-port elision, host/path normalization.
  - Acceptance: `Parse(Serialize(Parse(u))) == Parse(u)` for the test corpus.
  - Spec: URL Standard.
- **P1-URL-003** Origin and site computation.
  - Deliverable: `urls.Origin`, `urls.Site`, opaque origins, `SameOrigin`,
    `SameSite`; registrable domain via `x/net/publicsuffix`.
  - Acceptance: opaque/tuple origin equality tests; site tests against public-suffix
    vectors.
  - Spec: URL Standard, HTML origin (`specifications/html/origin.html`).
- **P1-URL-004** IDNA host parsing/serialization.
  - Deliverable: integration with `golang.org/x/net/idna`; forbidden-host and
    percent-decoding rules.
  - Acceptance: IDNA test vectors; failure cases map to the WHATWG host parser outcomes.
  - Spec: URL Standard.
- **P1-URL-005** Internal scheme handling.
  - Deliverable: `about:`, `data:`, `blob:`, `file:` (restricted), and `netsurf-cache:`
    recognition and canonicalization.
  - Acceptance: round-trip and policy tests.
  - Spec: URL Standard, HTML (`specifications/html/infrastructure.html`).
- **P1-ENC-001** Encoding sniffing.
  - Depends on: P1-URL-001.
  - Deliverable: BOM sniff, transport `Content-Type` charset, 1024-byte `<meta>` prescan,
    decode via `x/text/encoding`.
  - Acceptance: matches the WHATWG encoding-sniffing order on a fixture corpus.
  - Spec: Encoding Standard (`specifications/web/encoding.html`), HTML
    (`specifications/html/parsing.html`).
- **P1-ENC-002** Encoding change handling.
  - Deliverable: detection of a `<meta>`-declared encoding change and re-decode policy.
  - Acceptance: tests for UTF-16 stickiness and identical-prefix re-decode.
  - Spec: HTML parsing.
- **P1-MICRO-001** Numeric microsyntaxes.
  - Deliverable: signed/non-negative integers, floating point, percentages, dimensions.
  - Acceptance: HTML common-microsyntax vectors.
  - Spec: `specifications/html/common-microsyntaxes.html`.
- **P1-MICRO-002** Date/time microsyntaxes.
  - Deliverable: month, date, yearless date, time, local/global date-time, timezone
    offset, week, duration.
  - Acceptance: HTML vectors.
  - Spec: `specifications/html/common-microsyntaxes.html`.
- **P1-MICRO-003** Color, token-list, and reference microsyntaxes.
  - Deliverable: simple color, legacy color parsing, space/comma token lists, hash-name
    references.
  - Acceptance: HTML vectors.
  - Spec: `specifications/html/common-microsyntaxes.html`.

**Exit criteria:** `urls` is the only URL implementation used by any other package
(enforced by review), and encoding/microsyntax packages are fully unit tested.

---

## Phase 2 — Network Transport, Cookies, and Cache Store

**Goal:** fetch real pages over TLS/HTTP1.1 with hardened framing and an archive cache.

### Features
- Custom HTTP/1.1 client, TLS wrapper, DNS resolver.
- RFC 7234 cache store with aggressive archive semantics.
- Cookie jar with tracking-session scoping.
- SOCKS5, WebSocket, and internal protocol handlers.

### Tasks

- **P2-HTTP-001** HTTP/1.1 request serialization.
  - Deliverable: request line, header serialization with privacy ordering/casing, body
    framing, `Host` generation.
  - Acceptance: byte-exact golden tests; injection tests reject CR/LF in names/values.
  - Spec: RFC 7230 (`specifications/http/rfc7230.txt`).
- **P2-HTTP-002** HTTP/1.1 response parsing.
  - Deliverable: status line, header parsing (reject whitespace before colon, obs-fold
    policy), `Content-Length`/`Transfer-Encoding` precedence, chunked with trailers,
    body-until-close, `100-continue`, `101` upgrade.
  - Acceptance: framing vectors; smuggling corpora rejected.
  - Spec: RFC 7230, RFC 7231 (`rfc7231.txt`), RFC 6585 (`rfc6585.txt`).
- **P2-HTTP-003** Connection pool.
  - Deliverable: keep-alive, per-host cap, idle timeout, staged half-close, idempotent
    retry.
  - Acceptance: concurrency and reuse tests against a local test server.
  - Spec: RFC 7230.
- **P2-HTTP-004** Redirect engine.
  - Deliverable: 301/302/303/307/308 handling, method rewriting, loop detection, scheme
    downgrade blocking, referrer policy hooks.
  - Acceptance: redirect-matrix tests.
  - Spec: RFC 7231.
- **P2-HTTP-005** Range and conditional requests.
  - Deliverable: `Range`, `If-Range`, `If-None-Match`, `If-Modified-Since`.
  - Acceptance: RFC vectors; 206/304 handling.
  - Spec: RFC 7232 (`rfc7232.txt`), RFC 7233 (`rfc7233.txt`).
- **P2-HTTP-006** Content-Disposition and MIME handling.
  - Deliverable: `filename`/`filename*` parsing with sanitization; Content-Type parsing.
  - Acceptance: traversal and control-character rejection tests.
  - Spec: RFC 6266 (`rfc6266.txt`), MIME Sniffing (`specifications/web/mimesniff.html`).
- **P2-TLS-001** TLS configuration and connection.
  - Deliverable: TLS 1.3 (1.2 configurable), ALPN, SNI, per-site session cache, 0-RTT
    disabled, certificate validation and HSTS.
  - Acceptance: handshake tests; no insecure bypass path exists.
  - Spec: RFC 8446 (`specifications/other/rfc8446.txt`).
- **P2-TLS-002** ECH and GREASE.
  - Depends on: P2-TLS-001.
  - Deliverable: ECH configuration where supported; GREASE/ClientHello permutation via
    `protocols/tls/chrome` fork.
  - Acceptance: ClientHello byte tests for GREASE values; documented fork provenance.
  - Spec: RFC 8446, RFC 8701 (`specifications/other/rfc8701.txt`).
- **P2-DNS-001** DNS message codec.
  - Deliverable: header/question/RR parse-serialize, name compression with loop limits.
  - Acceptance: RFC 1035 vectors; compression-pointer loop rejection.
  - Spec: RFC 1034/1035 (`specifications/dns/rfc1034.txt`, `rfc1035.txt`).
- **P2-DNS-002** Resolver and cache.
  - Deliverable: CNAME handling, TTL cache, negative caching, special-use names,
    `localhost`/`.local` handling, EDNS(0).
  - Acceptance: resolver vectors; TTL/negative-cache tests.
  - Spec: RFC 1034/1035, RFC 2181 (`rfc2181.txt`), RFC 6761 (`rfc6761.txt`), RFC 6891
    (`rfc6891.txt`), RFC 3596 (`rfc3596.txt`).
- **P2-DNS-003** DoH and DoT transports.
  - Deliverable: `application/dns-message` GET/POST with base64url, padding, DoT
    length-prefix and pinning.
  - Acceptance: interoperability tests; no cookies sent to DoH endpoints.
  - Spec: RFC 8484 (`specifications/dns/rfc8484.txt`), RFC 7858 (`rfc7858.txt`),
    RFC 7830 (`specifications/other/rfc7830.txt`).
- **P2-CACHE-001** RFC 7234 cache engine.
  - Deliverable: freshness, age, validators, `Vary`, revalidation, 304 merge,
    invalidation.
  - Acceptance: RFC 7234 directive/directive-matrix tests.
  - Spec: RFC 7234 (`rfc7234.txt`).
- **P2-CACHE-002** Aggressive archive policy.
  - Deliverable: configurable policy that retains responses beyond server expiry,
    surfaces the archive banner, and revalidates only on explicit reload.
  - Acceptance: expired-but-archived response is served and flagged; reload refreshes.
  - Spec: RFC 7234 plus project policy (`cache.md`).
- **P2-CACHE-003** Disk mapping and journal.
  - Deliverable: `caches/disk` URL↔path mapping, `~q~`/`~vary~`/`~r~` sentinels,
    collision promotion, redirect stubs, atomic writes.
  - Acceptance: invertibility property tests; export without journal remains browsable.
  - Spec: project policy (`cache.md`).
- **P2-COOKIE-001** Cookie parsing and storage.
  - Deliverable: RFC 6265 attribute parsing, domain/path matching, expiry, `Secure`,
    `HttpOnly`, `SameSite`, prefixes, public-suffix rejection.
  - Acceptance: cookie vectors; partition tests.
  - Spec: RFC 6265 (`specifications/http/rfc6265.txt`), RFC 2109
    (`specifications/http/rfc2109.txt`).
- **P2-COOKIE-002** Tracking-session cookie model.
  - Deliverable: cross-site dependency graph, cookie collection/clearing on reload,
    CNAME-cloaked third-party defense (see `privacy.md`).
  - Acceptance: scenario tests prove a reload yields a fresh tracking session and that
    dependent cookies are cleared atomically.
  - Spec: project policy (`privacy.md`).
- **P2-SOCKS-001** SOCKS5 client.
  - Deliverable: negotiation, CONNECT, DOMAINNAME, auth.
  - Acceptance: handshake against a test SOCKS server.
  - Spec: RFC 1928 (`specifications/socks/rfc1928.txt`).
- **P2-WS-001** WebSocket client.
  - Deliverable: handshake, masking, fragmentation, control frames, close codes, UTF-8
    validation.
  - Acceptance: framing and handshake tests.
  - Spec: RFC 6455 (`specifications/ws/rfc6455.txt`).
- **P2-INTERNAL-001** Internal scheme handlers.
  - Deliverable: `data:`, `about:`, `netsurf-cache:` handlers.
  - Acceptance: handler tests and policy checks.
  - Spec: URL Standard.

**Exit criteria:** `netsurfctl fetch <url>` retrieves and caches a page over TLS/HTTP1.1
end to end, with cookies and archive semantics verified by tests.

---

## Phase 3 — HTML Parsing and DOM

**Goal:** turn bytes into a queryable, spec-shaped DOM.

### Features
- `x/net/html` integration with encoding and node budgets.
- `dom` tree and traversal APIs.
- Shadow DOM and template contents scaffolding.

### Tasks

- **P3-HTML-001** Parser wrapper and node budget.
  - Deliverable: parse entry points over `x/net/html` with depth/node/attribute budgets.
  - Acceptance: html5lib tree dumps mapped to `dom` match; budget tests reject bombs.
  - Spec: `specifications/html/parsing.html`.
- **P3-HTML-002** DOM tree construction from `x/net/html` nodes.
  - Deliverable: namespaces, attributes, text/comment/doctype, parent/child/sibling links,
    stable node IDs, tree order.
  - Acceptance: structural tests and html5lib corpus.
  - Spec: DOM Standard (`specifications/web/dom.html`), HTML parsing.
- **P3-DOM-001** Core DOM APIs.
  - Deliverable: `getElementById`, class/id indexes, `children`, `parentNode`,
    `textContent`, `innerHTML` (parse fragment).
  - Acceptance: API tests and fragment-parsing tests.
  - Spec: DOM Standard, HTML parsing.
- **P3-DOM-002** Template contents and shadow DOM scaffolding.
  - Deliverable: separate template `DocumentFragment`; slot assignment model.
  - Acceptance: template/host tests.
  - Spec: HTML (`specifications/html/dom.html`), DOM Standard.
- **P3-DOM-003** Origin, sandbox flags, and access checks on `Document`.
  - Deliverable: `Document.Origin`, sandboxing flag set, same-origin gate for DOM access.
  - Acceptance: sandbox-directive tests.
  - Spec: `specifications/html/origin.html`.
- **P3-DOM-004** Quirks mode and document lifecycle.
  - Deliverable: quirks/limited-quirks/no-quirks classification; `readyState` transitions.
  - Acceptance: DOCTYPE corpus tests.
  - Spec: HTML parsing.

**Exit criteria:** fetching and parsing a real page produces a DOM that can be traversed
and queried, with encoding, quirks, and budgets verified.

---

## Phase 4 — CSS Parser and CSSOM (the core pitfall)

**Goal:** a fully tokenized, lexed, specification-compliant CSS engine with an internal
object model.

### Features
- CSS Syntax 3 tokenizer with source representations.
- Parser for rules, declarations, and component values.
- Value-definition grammar engine.
- Selectors Level 4.
- Values and units including math functions and Color 4.
- CSSOM with serialization.
- Cascade with custom properties, layers, and conditions.
- Media queries and animations.

### Tasks

- **P4-CSS-001** Tokenizer.
  - Deliverable: preprocessing, all token kinds/flags, every consume algorithm, preserved
    `Representation`.
  - Acceptance: CSS Syntax tokenizer suite as goldens; An+B/`<urange>` representation
    tests.
  - Spec: `specifications/css/syntax-3.html`.
- **P4-CSS-002** Parser and component values.
  - Deliverable: at-rules, qualified rules, declarations, simple blocks, functions, all
    entry points.
  - Acceptance: parser suite goldens; error-recovery fixtures.
  - Spec: `specifications/css/syntax-3.html`.
- **P4-CSS-003** Value-definition grammar engine.
  - Deliverable: combinators (`&&`, `||`, `|`), multipliers, comma rules, range checks.
  - Acceptance: grammar tests across representative properties.
  - Spec: `specifications/css/values-and-units-4.html` §2.
- **P4-CSS-004** Selectors Level 4.
  - Deliverable: selector parsing, specificity, `An+B`, `:is`/`:where`/`:not`/`:has`,
    pseudo-classes/elements.
  - Acceptance: selector matching suite.
  - Spec: `specifications/css/selectors-4.html`.
- **P4-CSS-005** Values, units, and math.
  - Deliverable: numeric/relative units, angles/time/frequency/resolution, `calc()` and
    friends, `attr()`, `<ratio>`.
  - Acceptance: value grammar and calculation-tree simplification tests.
  - Spec: `specifications/css/values-and-units-4.html`.
- **P4-CSS-006** Color 4.
  - Deliverable: hex/named/rgb/hsl/hwb/lab/lch/`color()`/`currentColor` parsing and
    serialization.
  - Acceptance: color vector tests and serialization round-trips.
  - Spec: `specifications/css/color-4.html`.
- **P4-CSS-007** CSSOM.
  - Deliverable: `CSSStyleSheet`, rule types, `CSSStyleDeclaration`, `MediaList`,
    serialization.
  - Acceptance: CSSOM tests; parse→serialize→parse stability.
  - Spec: `specifications/css/cssom-1.html`, `syntax-3.html` §10.
- **P4-CSS-008** Cascade and inheritance.
  - Deliverable: origin/importance ordering, context, specificity, order, defaulting
    keywords, `all`, layers, custom properties and `var()` with cycle detection.
  - Acceptance: cascade tests including `revert` and custom-property cycles.
  - Spec: `specifications/css/cascade-4.html`, `specifications/css/cascade-5.html`,
    `specifications/css/variables-1.html`.
- **P4-CSS-009** Conditional rules.
  - Deliverable: `@media`, `@supports`, `@import`, condition grammar, three-valued logic.
  - Acceptance: `@supports` and `@import` tests.
  - Spec: `specifications/css/conditional-rules-3.html`.
- **P4-CSS-010** Media queries.
  - Deliverable: full grammar, `min-`/`max-`/range syntax, standard features, `Environment`
    evaluation.
  - Acceptance: media-query matrices across environments.
  - Spec: `specifications/css/media-queries-5.html`.
- **P4-CSS-011** Property registry.
  - Deliverable: name → grammar/inherited/initial/computed/animation metadata tables.
  - Acceptance: registry completeness tests against the supported property set.
  - Spec: relevant CSS module per property.
- **P4-CSS-012** Animations.
  - Deliverable: `@keyframes`, animation properties, timing, animation origin.
  - Acceptance: keyframe and timing tests.
  - Spec: `specifications/css/animations-1.html`.
- **P4-CSS-013** CSS Nesting.
  - Deliverable: nested rules and `&` resolution.
  - Acceptance: nesting tests.
  - Spec: `specifications/css/css-nesting-1.html`.
- **P4-CSS-014** Fuzzing and performance.
  - Deliverable: fuzz targets and benchmarks for tokenizer/parser/selectors/values.
  - Acceptance: no known panics; benchmarks recorded.
  - Spec: N/A.

**Exit criteria:** the CSSWG/WPT syntax, selector, values, and cascade test subsets pass,
and parse→serialize→parse is stable for the full corpus.

---

## Phase 5 — Style Resolution

**Goal:** compute used styles for every element.

### Features
- UA stylesheet and presentational hints.
- Selector matching and cascade application.
- Computed values, inheritance, custom properties, media/conditionals.
- Incremental restyle.

### Tasks

- **P5-STYLE-001** UA stylesheet and presentational hints.
  - Deliverable: embedded `useragent.css`; HTML presentational attribute mapping.
  - Acceptance: default-display matrix tests.
  - Spec: `specifications/html/rendering.html`.
- **P5-STYLE-002** Selector matching engine.
  - Deliverable: id/class/tag/attribute indexes; right-to-left matching; `:has()`.
  - Acceptance: matching correctness and performance tests.
  - Spec: `specifications/css/selectors-4.html`.
- **P5-STYLE-003** Computed value resolution.
  - Deliverable: per-property computed-value functions; relative unit resolution;
    inheritance.
  - Acceptance: computed-style golden tests.
  - Spec: property specifications plus `cascade-4.html`.
- **P5-STYLE-004** Custom properties and `var()` substitution.
  - Deliverable: inherited token streams, substitution at computed-value time, invalid-
    at-computed-value-time.
  - Acceptance: variable substitution and cycle tests.
  - Spec: `specifications/css/variables-1.html`.
- **P5-STYLE-005** Media/conditional evaluation integration.
  - Deliverable: `Environment` wired to viewport and user preferences.
  - Acceptance: dynamic media-change restyle tests.
  - Spec: `media-queries-5.html`, `conditional-rules-3.html`.
- **P5-STYLE-006** Incremental restyle.
  - Deliverable: dirty-element tracking and restyle invalidation.
  - Acceptance: mutation restyle correctness and performance tests.
  - Spec: N/A.

**Exit criteria:** for a corpus of real pages, `getComputedStyle`-equivalent values match a
recorded baseline and restyle is correct after DOM mutations.

---

## Phase 6 — Layout

**Goal:** modern full layout.

### Features
- Box tree with anonymous boxes and all display types.
- Formatting contexts: block, inline, table, flex, grid, float, absolute.
- Intrinsic sizing, margin collapsing, positioning, overflow.
- Writing modes, bidi, line breaking, and simple text metrics.

### Tasks

- **P6-LAYOUT-001** Box tree construction.
  - Deliverable: principal and anonymous boxes; `display` handling including `contents`.
  - Acceptance: box-tree golden tests.
  - Spec: `specifications/css/css-display-3.html`, `specifications/html/rendering.html`.
- **P6-LAYOUT-002** Box model and intrinsic sizing.
  - Deliverable: margins/borders/padding, percentages, min/max, replaced elements,
    `aspect-ratio`.
  - Acceptance: sizing tests.
  - Spec: `specifications/css/css-box-3.html`, `specifications/css/css-sizing-3.html`.
- **P6-LAYOUT-003** Block formatting context.
  - Deliverable: block layout, margin collapsing including quirks.
  - Acceptance: block layout tests.
  - Spec: `specifications/css/css2-2.html`.
- **P6-LAYOUT-004** Inline formatting context.
  - Deliverable: line boxes, inline-level boxes, vertical alignment, line breaking.
  - Acceptance: inline layout tests.
  - Spec: `specifications/css/css2-2.html`, `specifications/css/css-text-3.html`.
- **P6-LAYOUT-005** Table formatting context.
  - Deliverable: table grid, row/column spans, borders and border-collapse.
  - Acceptance: table layout tests.
  - Spec: `specifications/css/css2-2.html`, `specifications/html/tables.html`.
- **P6-LAYOUT-006** Floats.
  - Deliverable: float placement, clearance, float interaction with line boxes.
  - Acceptance: float tests.
  - Spec: `specifications/css/css2-2.html`.
- **P6-LAYOUT-007** Positioning.
  - Deliverable: relative/absolute/fixed/sticky, containing blocks, z-index.
  - Acceptance: positioning tests.
  - Spec: `specifications/css/css-position-3.html`.
- **P6-LAYOUT-008** Flexbox.
  - Deliverable: flex containers/items, flexing, alignment, ordering.
  - Acceptance: flexbox test subset.
  - Spec: `specifications/css/css-flexbox-1.html`.
- **P6-LAYOUT-009** Grid.
  - Deliverable: grid containers, track sizing, placement, alignment.
  - Acceptance: grid test subset.
  - Spec: `specifications/css/css-grid-2.html`.
- **P6-LAYOUT-010** Writing modes and logical properties.
  - Deliverable: horizontal/vertical modes, logical-to-physical mapping.
  - Acceptance: writing-mode tests.
  - Spec: `specifications/css/css-writing-modes-4.html`.
- **P6-LAYOUT-011** Bidi and text runs.
  - Deliverable: bidi reordering via `x/text/unicode/bidi`; run segmentation.
  - Acceptance: bidi tests.
  - Spec: `specifications/css/css-writing-modes-4.html`, `specifications/css/css-text-3.html`.
- **P6-LAYOUT-012** Text metrics and simple shaping.
  - Deliverable: font matching, advance widths, kerning, fallback.
  - Acceptance: metric tests; `TextShaper` interface with `SimpleShaper`.
  - Spec: `specifications/css/css-fonts-4.html`, `specifications/css/css-text-3.html`.
- **P6-LAYOUT-013** Overflow and scrolling.
  - Deliverable: overflow computation, scroll containers, clipping.
  - Acceptance: overflow tests.
  - Spec: `specifications/css/css-overflow-3.html`, CSSOM View.
- **P6-LAYOUT-014** Incremental layout.
  - Deliverable: dirty layout and reflow avoidance.
  - Acceptance: reflow correctness and performance tests.
  - Spec: N/A.

**Exit criteria:** a corpus of layout reftests renders with the correct box geometry and
the incremental path produces identical results to full layout.

---

## Phase 7 — Painting, Fonts, Images, Color

**Goal:** render laid-out boxes to pixels.

### Features
- Rasterizer and compositor.
- Backgrounds, borders, radius, shadows, border images.
- Image decoding including SVG subset.
- Fonts and glyph rasterization.
- Color management.

### Tasks

- **P7-PAINT-001** Rasterizer.
  - Deliverable: premultiplied surface, paths, anti-aliasing, fill rules, stroking,
    clipping, Porter-Duff.
  - Acceptance: raster golden tests.
  - Spec: N/A (implementation detail).
- **P7-PAINT-002** Backgrounds and borders.
  - Deliverable: background layers, `background-size/position/repeat/origin/clip`,
    borders, `border-radius`, `box-shadow`, `border-image`.
  - Acceptance: visual reftests.
  - Spec: `specifications/css/backgrounds-and-borders-3.html`.
- **P7-IMG-001** Image decoding.
  - Deliverable: PNG/JPEG/GIF (stdlib), WebP (`x/image/webp`), animated images, EXIF
    orientation, pixel budgets.
  - Acceptance: decode tests and bomb-rejection tests.
  - Spec: `specifications/css/images-4.html`.
- **P7-IMG-002** SVG subset renderer.
  - Deliverable: secure static SVG with shapes, paths, gradients, and transforms.
  - Acceptance: SVG reftests; script/foreignObject disabled.
  - Spec: SVG (add if needed), `specifications/css/images-4.html`.
- **P7-IMG-003** CSS gradients.
  - Deliverable: linear/radial/conic and repeating variants, color-stop fixup.
  - Acceptance: gradient reftests.
  - Spec: `specifications/css/images-4.html`.
- **P7-FONT-001** Font loading and rasterization.
  - Deliverable: `x/image/font/sfnt` + `x/image/font/opentype`; system font enumeration;
    glyph cache.
  - Acceptance: glyph raster tests across fallback chains.
  - Spec: `specifications/css/css-fonts-4.html`.
- **P7-FONT-002** `@font-face` rule parsing and descriptors.
  - Depends on: P4-CSS-007, P7-FONT-001.
  - Deliverable: an `@font-face` rule object in the CSSOM and parsing/serialization
    for `font-family`, `src` (`url()`/`local()`/`format()`), `font-style`,
    `font-weight`, `font-stretch`, `unicode-range`, `font-display`,
    `font-feature-settings`, `font-variation-settings`, `size-adjust`,
    `ascent-override`, `descent-override`, and `line-gap-override`. Invalid
    descriptors and sources are rejected without discarding the whole rule.
  - Acceptance: CSS Fonts 4 `@font-face` parsing corpus; invalid-descriptor
    rejection; parse→serialize→parse stability; rule mutation tests.
  - Spec: `specifications/css/css-fonts-4.html`.
- **P7-FONT-003** Web-font loading, caching, and `font-display`.
  - Depends on: P7-FONT-002; the Phase 9 network broker for production fetching.
  - Deliverable: a font resource loader for raw TTF/OTF and WOFF (WOFF2 requires
    a Brotli decoder and is explicitly out of scope until one is added); `local()`
    lookup; a partitioned cache keyed by URL + descriptor set; `font-display`
    (`auto`, `block`, `swap`, `fallback`, `optional`) behavior; load failures
    falling back to the next `src`/system face. Network access goes through the
    renderer's fetch provider until the broker lands, and cache keys include the
    top-level site partition.
  - Acceptance: loader/cache tests with local HTTP fixtures; `font-display`
    timing tests; cross-site cache-isolation tests; malformed/oversized font
    rejection; fallback-on-failure tests.
  - Spec: `specifications/css/css-fonts-4.html`,
    `specifications/web/fetch.html`, `privacy.md`.
- **P7-FONT-004** Author-font matching and text features.
  - Depends on: P7-FONT-002, P7-FONT-003.
  - Deliverable: extend the font library with dynamically registered author
    faces; matching across family/weight/stretch/style/`unicode-range`; per-face
    glyph fallback; `font-feature-settings`, `font-variation-settings`, and
    variable-axis selection; metric overrides. System and author faces share
    one deterministic fallback chain.
  - Acceptance: matching matrices, `unicode-range` fallback tests, variable-font
    axis tests, feature-tag tests, and golden text rendering with a custom face.
  - Spec: `specifications/css/css-fonts-4.html`.
- **P7-COLOR-001** Color management.
  - Deliverable: sRGB/display-p3/lab/lch conversions; premultiplied blending.
  - Acceptance: color conversion vectors.
  - Spec: `specifications/css/color-4.html`.
- **P7-PAINT-003** Paint and stacking.
  - Deliverable: stacking contexts, z-order, transforms, replaced content, text painting.
  - Acceptance: stacking and transform reftests.
  - Spec: `specifications/css/css2-2.html`, CSS Transforms (add if needed).
- **P7-COMPOSITE-001** Compositor and damage tracking.
  - Deliverable: layerization, tile cache, damage rectangles, CPU compositing.
  - Acceptance: damage tests and end-to-end frame timing.
  - Spec: N/A.
- **P7-PAINT-004** Headless renderer and pixel test harness.
  - Deliverable: render a document to PNG; WPT reftest comparator.
  - Acceptance: pixel tests run in CI.
  - Spec: N/A.

**Exit criteria:** a page renders to a PNG that matches recorded baselines within the
pixel-diff tolerance, and the reftest harness runs in CI.

---

## Phase 11 — Rendering Backends

**Goal:** support multiple rendering pipelines behind backend-neutral interfaces.

### Tasks

- **P11-RENDER-001** Geometry, interfaces, and shared CSS helpers.
  - Deliverable: `rendering/geometry`, `rendering/interfaces`, `rendering/utils/css`;
    move shared types and pure math; define `Surface`, `Canvas`, `Renderer`,
    `Backend`, `Presenter`, and `Scene`.
  - Acceptance: all existing tests pass; no behavior change.
  - Spec: N/A.
- **P11-RENDER-002** Scene recorder and software parity.
  - Deliverable: painter records `interfaces.Canvas` commands; software executor
    replays scenes; parity test compares direct and recorded pixels.
  - Acceptance: byte-identical software output.
  - Spec: N/A.
- **P11-RENDER-003** Move software backend to `rendering/software`.
  - Deliverable: surface, rasterizer, compositing, image sampling, and SVG move
    behind `interfaces.Surface`; render APIs return `(Surface, error)`.
  - Acceptance: full test suite green.
  - Spec: N/A.
- **P11-RENDER-004** Backend selection.
  - Deliverable: `rendering/backends`, `--renderer` on `cmd/netsurf` and
    `netsurfctl`, renderer plumbing through pages, subframes, chrome, and reftests.
  - Acceptance: software default; unavailable Vulkan reports a clear error.
  - Spec: N/A.
- **P11-RENDER-005** Vulkan GPU backend.
  - Deliverable: cgo bindings, offscreen targets/readback, shader pipelines for
    all canvas commands, X11 swapchain presenter.
  - Acceptance: cross-backend parity suite within tolerance; GPU benchmarks.
  - Spec: N/A.

**Status:** `P11-RENDER-001`–`004` are implemented and verified. `P11-RENDER-005`
is implemented for the offscreen GPU path: solid fills, CPU coverage masks,
gradients, images, glyphs, masks, blend-pixel groups, and opacity/masked layers
render through Vulkan and are read back into a surface. Non-identity transformed
layers fall back to software; GPU path/gradient/glyph pipelines and the X11
swapchain presenter remain. See [`rendering-refactor.md`](../rendering-refactor.md).

---

## Phase 8 — Window and UI Shell

**Goal:** an interactive native browser.

### Features
- Pure-Go X11 backend.
- Browser chrome and tab model.
- Event loop, input, clipboard, DPI.

### Tasks

- **P8-X11-001** X11 connection and window.
  - Deliverable: XCB core, atoms, window creation, map/configure, expose events.
  - Acceptance: window opens and presents a surface on X11.
  - Spec: X11 protocol (implementation detail).
- **P8-X11-002** X11 input and clipboard.
  - Deliverable: key/pointer events, keysym mapping, clipboard.
  - Acceptance: input and clipboard tests.
  - Spec: X11 protocol.
- **P8-X11-003** MIT-SHM surface upload.
  - Deliverable: shared-memory image upload and damage presentation.
  - Acceptance: performance test and visual verification.
  - Spec: X11 protocol.
- **P8-SHELL-001** Browser chrome.
  - Deliverable: tab strip, omnibox, navigation, status, and privacy dashboard rendered
    with our rasterizer.
  - Acceptance: UI interaction tests.
  - Spec: N/A.
- **P8-SHELL-002** Tab/session/navigation model.
  - Deliverable: `browsers` tab model, history, session restore.
  - Acceptance: navigation and history tests.
  - Spec: HTML browsing-context model.
- **P8-SHELL-003** Event loop and input routing.
  - Deliverable: vsync-driven loop, input routing to renderer, keyboard shortcuts.
  - Acceptance: end-to-end interaction test.
  - Spec: N/A.

**Exit criteria:** a user can launch `netsurf`, type a URL, and browse a real site with
correct rendering.

---

## Phase 9 — Process Architecture and Sandboxing

**Goal:** move the renderer into a sandboxed process and the network into a broker.

### Features
- Broker/renderer split over `ipc`.
- Linux sandbox (namespaces, seccomp, Landlock, rlimits, cgroups).
- Shared-memory surfaces.
- Crash recovery and supervision.

### Tasks

- **P9-IPC-001** Framing and capability protocol.
  - Deliverable: versioned message framing, request IDs, cancellation, capability tokens.
  - Acceptance: fuzz and interoperability tests.
  - Spec: N/A.
- **P9-IPC-002** Shared memory.
  - Deliverable: `memfd`/POSIX shm surfaces with fd passing.
  - Acceptance: zero-copy surface tests.
  - Spec: N/A.
- **P9-RT-001** Process supervisor.
  - Deliverable: spawn/re-exec renderers and the broker, watchdogs, restart policy.
  - Acceptance: kill/restart tests.
  - Spec: N/A.
- **P9-SEC-001** Sandbox primitives.
  - Deliverable: user/net/mount/pid namespaces, no-new-privs, capability drop, seccomp
    allow-list, Landlock rules, rlimits, cgroup limits.
  - Acceptance: escape-attempt tests; verified absence of network/FS access.
  - Spec: `security.md`, Linux documentation.
- **P9-BROKER-001** Network broker.
  - Deliverable: all sockets and cache/cookie/storage access behind capability-scoped IPC.
  - Acceptance: renderer cannot open sockets or read the cache directory directly.
  - Spec: `security.md`.
- **P9-BROKER-002** Storage broker.
  - Deliverable: partitioned origin storage and settings behind IPC.
  - Acceptance: partition isolation tests.
  - Spec: `privacy.md`.

**Exit criteria:** a compromised renderer cannot reach the network or the cache, and
renderer crashes are isolated and recoverable.

---

## Phase 10 — JavaScript, Web APIs, and Additional Platforms

**Goal:** scriptable pages with privacy farbling, plus Wayland and broader CSS.

### Features
- Vendored, dependency-pruned `sobek`.
- Pluggable/activatable/farble-able engine.
- Web API bindings with farbling.
- Wayland backend.
- Animations, paged media, HTTP/2, complex shaping.

### Tasks

- **P10-JS-001** Vendor `sobek` fork.
  - Deliverable: `javascripts/sobek` with upstream revision recorded and non-essential
    dependencies removed; RegExp reimplemented on stdlib `regexp`.
  - Acceptance: sobek test suite passes with pruned dependencies.
  - Spec: ECMAScript (add spec reference).
- **P10-JS-002** Engine registry and activation.
  - Deliverable: `engines/javascripts` interface, registry, feature flag.
  - Acceptance: scripts inert when disabled; `<noscript>` rendered.
  - Spec: HTML scripting (`specifications/html/interactive-elements.html`).
- **P10-JS-003** Event loop and task scheduling.
  - Deliverable: task/microtask queues integrated with the renderer event loop.
  - Acceptance: event-ordering tests.
  - Spec: HTML webappapis.
- **P10-API-001** Core Web APIs.
  - Deliverable: window/document/navigator/screen/canvas/WebGL/audio/fonts/performance.
  - Acceptance: API conformance subset.
  - Spec: relevant WHATWG/W3C specs.
- **P10-API-002** Farble-able bindings.
  - Deliverable: every observable value routed through `farbling.Profile`.
  - Acceptance: fingerprint-consistency tests per origin.
  - Spec: `privacy.md`.
- **P10-WAYLAND-001** Wayland backend.
  - Deliverable: `wl_display`, `xdg_shell`, `wl_shm`, `wl_seat` client.
  - Acceptance: window opens and presents on a Wayland compositor.
  - Spec: Wayland protocol.
- **P10-CSS-001** Animations runtime.
  - Deliverable: tick-driven animation engine and events.
  - Acceptance: animation timing tests.
  - Spec: `specifications/css/animations-1.html`.
- **P10-CSS-002** Paged media.
  - Deliverable: `@page`, margin boxes, breaks.
  - Acceptance: paged-media tests.
  - Spec: `specifications/css/paged-media-3.html`.
- **P10-NET-001** HTTP/2.
  - Deliverable: multiplexed transport with privacy controls.
  - Acceptance: interop tests.
  - Spec: RFC 7540/9113 (add if needed).
- **P10-TEXT-001** OpenType shaping.
  - Deliverable: GSUB/GPOS shaper implementing `TextShaper`.
  - Acceptance: shaping tests for complex scripts.
  - Spec: OpenType (add if needed).

**Exit criteria:** a scripted, privacy-farbled page renders and behaves correctly on both
X11 and Wayland.

---

## Cross-cutting feature backlog

These features are required by the phases above but span multiple subsystems. They are
tracked separately so they are not lost:

- **X-ACC-001** Accessibility tree and screen-reader interface.
- **X-FORM-001** Forms, constraints, and submission.
- **X-SRCH-001** Find-in-page.
- **X-DL-001** Downloads with `Content-Disposition` sanitization.
- **X-PRINT-001** Printing pipeline.
- **X-PROFILE-001** Profile/config management.
- **X-LOCAL-001** UI localization and font fallback by locale.
- **X-DEV-001** Developer tools (DOM/CSS/network inspector), privacy-redacted.
