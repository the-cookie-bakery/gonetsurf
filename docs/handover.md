# Handover — Current Project State

_Last updated: 2026-10-28._

This document is the entry point for a new contributor. It describes what exists today,
how to build and verify it, what is intentionally incomplete, and where to start next.
The design intent lives in the other documents in this folder; this one is the honest
snapshot.

## 1. What this is

`gonetsurf` is a pure-Go web browser engine. The module path is `gonetsurf` (no host
prefix) and the target Go version is 1.27. Runtime dependencies are limited to the
standard library and `golang.org/x/*`; in-tree forks are permitted and must record their
upstream revision.

Work is organized into Phases 0–10 (see [`implementation-plan.md`](./implementation-plan.md))
and tracked task-by-task in [`task-tracking.md`](./task-tracking.md).

## 2. Current status

| Phase | Scope | State |
|---|---|---|
| 0 | Foundations (module, deps policy, panic barrier, fuzzing, docs) | **Done** (5/5 Verified) |
| 1 | WHATWG URL, encoding sniffing, HTML microsyntaxes | **Done** (10/10 Verified) |
| 2 | HTTP/1.1, TLS, DNS, cache, cookies, SOCKS, WebSocket, internal schemes | **Done** (18 Verified, 1 Implemented/Partial) |
| 3 | HTML parsing (`x/net/html`) + `dom` | **Done** (6/6 Verified) |
| 4 | CSS tokenizer → parser → selectors → values → CSSOM → cascade | **Done** (14/14 Verified) |
| 5 | Style resolution, UA sheet, selector matching, computed values | **Done** (6/6 Verified) |
| 6 | Layout: box tree, box model, block/inline/table/flex/grid, positioning, text | **Done** (14/14 Verified) |
| 7 | Paint/fonts/images/color | **Mostly done** (12 Verified, 1 Partial: OpenType feature/variation shaping) |
| 8 | X11 window/UI shell | **Done** (6/6 Verified) |
| 9 | Process sandbox/brokers | **Done** (6/6 Verified) |
| 10 | JS/Web APIs | **Not started** |
| X | Cross-cutting backlog (forms, downloads, print, a11y, devtools) | **Not started** |

Totals: **97 Verified, 2 Implemented/Partial, 18 Planned, 0 Blocked**. There are no open
bugs recorded.

There is now an interactive browser executable: `go run ./cmd/netsurf [url]` opens an
X11 window with a rasterized tab strip, omnibox, status/privacy bars, keyboard/mouse
routing, clipboard support, and real page rendering. Phases 0–7 provide the engine,
Phase 8 adds the X11 input/clipboard/MIT-SHM shell, full tab/navigation/browsing-context
model, page scrolling, omnibox search/autocomplete, inline iframes, tab reordering, and
the privacy dashboard, and Phase 9 puts network/cookie/cache/storage access behind a
separate broker and sandboxes the shell with a read-only Landlock view and INET-socket
seccomp guard.

## 3. Quick start

```sh
go build ./...                          # compile everything
go vet ./...                            # static checks
gofmt -l cmd urls parsers protocols caches storages internal dom styles layouting rendering windowing browsers ipc runtime brokers shells adapters   # formatting
go test ./... -count=1                  # unit + integration tests
go run ./cmd/netsurfctl deps-check      # enforce the no-third-party policy
go run ./cmd/netsurfctl urlparse 'https://www.example.com:8443/a/b?x=1#f'
go run ./cmd/netsurfctl layout page.html 800 600
go run ./cmd/netsurfctl render page.html page.png 800 600   # headless paint
go run ./cmd/netsurfctl show page.html 800 600               # paint in an X11 window
go run ./cmd/netsurf https://example.com/                    # interactive browser shell
go test ./urls/ -run TestURLParserConformance -v
go test ./parsers/css/tokenizer/ -run TestTokenizerConformance -v
go test ./urls/ -run='^$' -fuzz='^FuzzParse$' -fuzztime=30s
```

Current verification baseline:

- 315 Go files across 54 packages, 129 test files, 581 test/fuzz/benchmark functions.
- `go test ./...` green for all packages; `go vet ./...` clean; `gofmt` clean.
- WHATWG URL conformance: **896/896** `urltestdata.json` cases pass.
- CSS tokenizer conformance: **223/223** reference tokenization cases pass.
- CSS parser conformance: all `css-parsing-tests` entry points pass. Declaration-value
  whitespace is normalized to the current syntax; two non-ASCII identifier cases are
  documented skips (`parsers/css/parser/testdata/SKIPS.md`).
- Selectors: **270** WPT validity cases pass with 78 tentative-feature skips.
- Property registry: **353** properties are generated from the pinned specification
  propdef tables, WPT grammar assertions, and explicit overrides (`cmd/propertygen`).
- Property values: the stable Phase 4 WPT set (**2,780** cases) reports **2,520 passes,
  0 failures, 260 documented skips**. Skips are listed in
  `parsers/css/properties/testdata/SKIPS.md` (Color 5, experimental sizing/overflow,
  tentative grid-lanes, and deferred deep shorthand/image-function validation).
- Values/math: a 29-case CSS Values 4 math corpus passes, along with unit-conversion
  tests for absolute lengths, angles, and times.
- Fuzzing: new targets cover the grammar engine, selectors, values/math, CSSOM, and
  property validation; short runs completed >190k executions each without a crash.
- Benchmarks cover tokenization, parsing, selectors, colors, grammar matching, values,
  CSSOM serialization, property validation, and the cascade.
- Conditions: `@supports`/`@import` corpus passes, including nested `not`, selector
  validation, layers, and media preludes.
- Media queries: environment matrix covers range syntax, boolean features, color,
  monochrome, grid, pointer/hover, preferences, display mode, dynamic range, scripting,
  gamut, and viewport segments.
- Animations: WPT-derived shorthand corpus passes; keyframes, lists, timing, and
  conflict validation are tested.
- Nesting: Go-native flattening corpus passes for `&`, implicit nesting, selector lists,
  and nested declarations.
- CSSOM: `CSSStyleSheet`, `CSSStyleDeclaration`, `MediaList`, rule insertion/deletion,
  nested rules, selector-text mutation, and serialization pass their tests.
- Cascade/variables: custom-property inheritance, `var()` fallback/cycles, `all`,
  `revert`, and `revert-layer` pass the Go-native harness ported from WPT cascade and
  variables cases.
- Selector matching: the full Selectors Level 4 matcher is wired into `dom.Matches`,
  `QuerySelector`, and `QuerySelectorAll`; combinators, attributes, structural and
  logical pseudo-classes, `:has()`, `:scope`, languages, and states are covered.
- Style resolution: `styles.Matcher` maintains id/class/tag/attribute indexes and
  document order; the style engine applies the embedded UA sheet, presentational
  hints, author/inline origins, nesting, `@media`/`@supports`, inheritance, `all`,
  and custom properties/`var()`.
- Computed values: font-size keywords/`em`/`rem`/viewport units, font-weight
  bolder/lighter, line-height percentages, color/`currentcolor`, opacity, border
  widths, and generic length/angle/time conversion are tested.
- Incremental restyle: DOM mutation versions and dirty-node tracking drive cached
  subtree restyles; `:has()` stylesheets fall back to a full restyle.
- Layout: box-tree construction, anonymous block boxes, `display:contents`,
  multi-keyword `display` (`block flex`, `block flow-root`), generated
  `::before`/`::after` content boxes with logical inline ordering and box-model
  margins/padding/borders, box model/sizing
  (min/max width and height, percentages, `aspect-ratio`, `fit-content()`,
  `calc()` against containing blocks, intrinsic min-/max-content, horizontal
  auto margins), fixed/absolute flex formatting, block layout with positive,
  negative, collapsed-through, sibling, and parent/child margin collapsing,
  line-based inline formatting (wrapping, `text-align`, `vertical-align`,
  `white-space`, inline-blocks), and line-level bidi ordering are tested.
  Tables cover spans, column sizes, border-collapse, row groups, and cell
  alignment. Floats cover side-by-side placement, clearance, shrink-to-fit
  widths, and block avoidance. Positioning covers relative/absolute/fixed,
  padding-box containing blocks, static-position fallback, and sticky offsets.
  Flexbox covers row/column flow, wrapping, grow/shrink, gaps, order, alignment,
  baselines, and min/max main sizes. Grid covers explicit/repeat/implicit/minmax
  tracks, spans, auto-placement, item alignment, and percentage sizing. Text
  measurement uses a pluggable `TextShaper` with generic-family metrics,
  letter/word spacing, and pair kerning. Scroll containers expose
  scrollbar-aware scrollports, clipping flags, and clamped scroll offsets.
  Logical properties map through `writing-mode`/`direction`, vertical block
  flow honors block-axis margins, and text rotation metadata is recorded.
  `DocumentLayout` reuses clean subtree geometry for style-only mutations and
  verifies incremental output against full layout. A 35-case layout conformance
  harness (`layouting/testdata/cases.json`) passes all cases, and
  `netsurfctl layout <file|url>` provides an end-to-end parse/style/layout
  geometry dump.
- Rendering: the `rendering` package has premultiplied RGBA8 surfaces, anti-aliased
  nonzero/even-odd path filling, rounded rectangles, stroking with caps and
  joins, clipping, Porter-Duff compositing, and CSS color parsing. Linked
  stylesheets and nested `@import` rules are loaded through `adapters`, and
  relative resource URLs in author CSS are absolutized before parsing. The painter
  draws multi-layer backgrounds (color and data-URL images with repeat,
  position, size, origin, and clip), all CSS border styles with radii,
  per-side colors, outer/inset box shadows with blur, and bilinear image
  sampling. Replaced content is clipped to the curved content edge, non-visible
  overflow is clipped to the rounded padding edge, and rounded border arcs keep
  the border ring intact through the curves, following CSS Backgrounds and
  Borders 3 §4.2–4.3 and WPT `border-radius-clip-001`. Raster and feature goldens compare PNGs with `internal/pixeltest`,
  and `FuzzRaster` plus fill/stroke/blit benchmarks cover the hot paths.
- Images: `rendering/images` decodes PNG/JPEG/GIF/WebP with GIF frame
  composition and delays, applies JPEG EXIF orientation, and enforces
  pixel/dimension/frame budgets. `rendering.URLImageProvider` resolves relative
  URLs and supports `data:` plus caller-supplied HTTP(S) fetching; the
  `netsurfctl` render/show commands wire it to the HTTP client, so external
  background images render.
- Color: `parsers/css/colors` converts sRGB, linear sRGB, Display P3, Adobe RGB,
  ProPhoto, Rec. 2020, XYZ D50/D65, Lab/LCH, and OKLab/OKLCH to sRGB using the
  Color 4 matrices and transfer functions. `RGBA8` clips out-of-gamut values;
  the painter accepts these colors everywhere a CSS color is allowed.
- Gradients: linear, radial, conic, and repeating variants render with
  angle/corner directions, circle/ellipse extents, positions, color-stop fixup,
  and premultiplied sRGB interpolation. `rendering/testdata/golden/gradients.png`
  is the visual baseline and `FuzzGradient` covers malformed gradient syntax.
- Fonts: `rendering/fonts` matches embedded Go fonts and lazily indexed system
  TrueType/OpenType files by family/weight/style, provides per-rune fallback,
  real ascent/descent/advances and kerning through `layouting/model.TextShaper`, and
  caches rasterized glyph masks. The renderer uses the same library for layout,
  measurement, and painting. Shaping and glyph-cache benchmarks cover the text
  hot paths.
- Stacking and transforms: paint order follows stacking contexts with negative,
  zero/auto, and positive z-index groups; ancestors' clips and scroll offsets
  are carried into nested contexts. Opacity and 2D transforms (translate,
  scale, rotate, skew, matrix, and `transform-origin`) render through an
  offscreen layer with bilinear compositing. `<img>` elements draw through the
  image provider with object-fit modes, and a stacking/transform/replaced
  golden is the visual baseline. `FuzzTransformFunction` covers transform
  parsing.
- SVG: static SVG documents render through `rendering.DecodeSVG` with shapes,
  paths (including arcs), groups, viewBox/preserveAspectRatio, transforms,
  fill/stroke rules, and linear/radial gradients. Scripts, foreignObject,
  event handlers, and external references are ignored; node/pixel budgets are
  enforced. SVG works as an `<img>` source and CSS background image.
- Compositor: `Damage`, `Compositor`, and `TileGrid` provide damage merging,
  layered CPU compositing with opacity, and independently invalidated tiles;
  frame and tile benchmarks are recorded.
- Reftests: `internal/reftest` and `netsurfctl reftest` render test/reference
  HTML documents and compare them with the pixel harness.
- IPC/supervision: `ipc` provides versioned framing, cancellation, capability
  tokens, memfd shared memory, and fd passing; `runtime` supervises processes
  with restart/backoff/stop policies; `runtime/sandbox` provides rlimits,
  no-new-privs, capability dropping, namespaces, cgroups, Landlock, and seccomp
  primitives. Landlock denial and kill/restart behavior are tested.
- Brokers: `cmd/netsurf` spawns a separate broker process on private Unix
  sockets. `brokers/network` performs capability-scoped HTTP fetches with a
  partition-keyed Vary-aware response cache and persistent cookies;
  `brokers/storage` provides a persistent partition/origin-isolated key-value
  store. The shell's page/resource fetches and session persistence go through
  the brokers, then it installs an unprivileged sandbox baseline (rlimits,
  no-new-privs, capability drop, read-only Landlock) plus an AF_INET/AF_INET6
  seccomp guard.
- Headless paint: `rendering.RenderDocument` drives parse → style → layout → paint,
  and `netsurfctl render` writes the result to PNG. The E2E golden
  (`rendering/testdata/golden/page.png`) exercises backgrounds, borders, and
  real-font text.
- X11: `windowing/x11` is a pure-Go X11 client (Xauthority MIT-MAGIC-COOKIE-1,
  setup handshake, window/title/WM_DELETE, expose/configure handling, key/pointer
  events with keyboard mapping, ICCCM clipboard, MIT-SHM uploads with damage
  bands, and PutImage fallback). `netsurfctl show` opens a page in an X11 window;
  integration tests run when `DISPLAY` is set.
- Shell: `browsers` provides tabs, opener/child browsing contexts, frame
  history, same-document navigation, scroll restoration, tab reordering, and
  session restore; `shells` renders the tab strip/omnibox/status/privacy chrome
  with the rasterizer and routes input in a 60 Hz dirty-frame loop. The shell
  scrolls pages and nested scroll containers under the pointer, follows
  links/fragments, loads and paints inline iframes with sandbox navigation
  checks, completes URLs from history, falls back to DuckDuckGo search, and
  exposes a broker-backed privacy dashboard with tracking-session reset. Chrome
  surfaces, font-family lookups, text colors, per-rune font fallback, and scroll
  blits are cached; offscreen text is skipped before shaping and clipped
  overflow no longer expands the document scroll range, keeping fixed chrome in
  place. Nested scroll containers apply their scroll offset exactly once while
  painting, so scrolled-aside and scrolled-main content stays visible with its
  backgrounds and geometry. Window resize re-runs media queries, style, layout,
  and paint. A
  representative long-page scroll frame stays under the 60 Hz budget.
  `cmd/netsurf` is the interactive executable. Tests
  cover model navigation, shell rendering, keyboard/mouse routing, scrolling,
  link/fragment/iframe navigation, autocomplete, tab reordering, privacy
  actions, headless Run/close, X11 input, clipboard across two clients, SHM
  visual uploads, and damage presentation.
- Matching benchmarks: `dom` selector matching and `styles.Matcher.Elements` are
  benchmarked; the DOM matcher also has a fuzz target.
- An+B conformance: **128/128** cases; `<urange>`: **12/12** cases.
- Color 4: hex, keyword, HSL, HWB, lab/lch/oklab/oklch, and `color()`
  corpora pass (**~7,300** cases).
- IDNA: **2669** cases pass; encoding sniffing: **81** cases pass.
- HTML tree construction: **1578** cases pass (192 fragment contexts plus
  documented parser divergences skipped).
- HTTP/1.1: framing/smuggling corpus and a `net/http` differential test pass;
  `FuzzReadResponse` ran 1.7M executions without a crash.
- DNS-over-TLS: local TLS DNS server test passes. WebSocket: opening handshake,
  fragmentation, control frames, close codes, masking, and UTF-8 validation pass.
- `netsurfctl fetch <url>` retrieves a URL end to end.
- Verification is run through `make compliance-tests`.
- Fuzzing: `FuzzCSS` ran 375k executions without a crash.
- `deps-check` confirms only `golang.org/x/net` and `golang.org/x/text` (plus stdlib) are
  in the dependency graph.

## 4. Repository map (implemented)

| Path | What it does |
|---|---|
| `urls/` | WHATWG URL parser/serializer, origin, site, IDNA, internal schemes, `data:` decoding. |
| `parsers/html/` | `x/net/html` wrapper: encoding → parse → `dom` conversion, node budget, quirks detection, fragment parsing/serialization. |
| `parsers/html/encodings/` | BOM/transport/meta-prescan encoding sniffing and re-decode. |
| `parsers/html/microsyntaxes/` | HTML numeric, date/time, color, token-list, and reference parsers. |
| `parsers/css/tokenizer/` | CSS Syntax 3 tokenizer: preprocessing, all token kinds/flags, per-token `Representation`. |
| `parsers/css/parser/` | Component values, functions, simple blocks, rules, declarations, entry points. |
| `parsers/css/grammar/` | Value-definition grammar engine (`\|`, `\|\|`, `&&`, multipliers, ranges). |
| `parsers/css/selectors/` | Selectors Level 4 parsing, specificity, and the An+B microsyntax. |
| `parsers/css/values/` | Numeric units, unit conversion, `calc()`/`min()`/`max()`/`clamp()`, `<urange>`. |
| `parsers/css/colors/` | Color 4 parsing/serialization: hex, named, rgb/hsl/hwb, lab/oklch/color(). |
| `parsers/css/cssom/` | Stylesheet, style rule, at-rule, and declaration objects with serialization. |
| `parsers/css/cascade/` | Origin/importance/layer/specificity/order cascade and inheritance. |
| `parsers/css/conditions/` | `@supports` conditions and `@import` descriptors. |
| `parsers/css/mediaqueries/` | Media Queries 5 parsing and environment evaluation. |
| `parsers/css/properties/` | Property registry: inheritance, initial, grammar, animation metadata. |
| `parsers/css/animations/` | `@keyframes` and the animation shorthand. |
| `parsers/css/nesting/` | CSS Nesting parsing and `&` resolution. |
| `dom/` | Namespace-aware mutable DOM: nodes, documents, attributes, queries, templates, shadow scaffolding, sandbox flags. |
| `styles/` | Indexed selector matcher, UA stylesheet, presentational hints, and the style-resolution engine. |
| `layouting/` | Box model, software/Vulkan layout backends, sizing, positioning, text metrics, overflow, and incremental layout. |
| `rendering/` | Surfaces, path rasterization, stroking, clipping, Porter-Duff compositing, paint traversal, color conversion, and headless render. |
| `rendering/images/` | PNG/JPEG/GIF/WebP decoding, GIF animation metadata, EXIF orientation, and pixel/frame budgets. |
| `rendering/fonts/` | Embedded/system font matching, text shaping, metrics, per-rune fallback, and a glyph mask cache. |
| `adapters/` | Bridges between subsystems; currently linked/imported stylesheet loading and CSS resource-URL resolution. |
| `windowing/` | Display/window abstraction, input/clipboard events, headless backend, and the pure-Go X11 presenter with MIT-SHM. |
| `browsers/` | Tab, navigation/history, and session model. |
| `shells/` | Rasterized browser chrome, input routing, and the dirty-frame event loop. |
| `ipc/` | Versioned framing, capability tokens, memfd shared memory, and fd passing. |
| `runtime/` | Process supervisor with restart/backoff/stop policies. |
| `runtime/sandbox/` | Linux rlimits, namespaces, capabilities, Landlock, cgroups, and seccomp primitives. |
| `brokers/network/` | Capability-scoped HTTP fetch broker over IPC. |
| `brokers/storage/` | Partition/origin-isolated storage broker over IPC. |
| `internal/pixeltest/` | Golden/reference image comparison used by rendering tests. |
| `protocols/http1/` | Custom HTTP/1.1 client: ordered headers, strict framing, chunked+trailers, pool, redirects, conditional/range. |
| `protocols/tls/` | TLS 1.3 client config, ALPN, ECH field, GREASE values. |
| `protocols/dns/` | DNS codec (compression), caching resolver, special-use names, DoH, DoT. |
| `protocols/socks/` | SOCKS5 CONNECT client. |
| `protocols/websockets/` | RFC 6455 accept key and masked frame codec. |
| `protocols/mime/` | Content-Disposition parsing and filename sanitization. |
| `protocols/internal/` | `about:`, `data:`, `netsurf-cache:` handlers. |
| `caches/disk/` | Invertible URL↔filesystem mapping and JSONL journal/redirect chain. |
| `caches/http/` | RFC 7234 policy plus the aggressive archive mode. |
| `caches/memory/` | Generic in-memory LRU. |
| `storages/cookies/` | RFC 6265 partitioned jar, cross-site dependency graph, CNAME-cloaking tracking-session reset. |
| `internal/safecall/` | Panic-to-error boundary helpers. |
| `cmd/netsurf/` | Interactive X11 browser shell. |
| `cmd/netsurfctl/` | `deps-check`, `urlparse`, `fetch`, `layout`, `render`, `show`, and `reftest` diagnostic commands. |
| `cmd/corpusgen/` | Refresh and normalize the vendored WPT, URL, HTML, and CSS corpora. |
| `cmd/propertygen/` | Generate the property registry from pinned specs and WPT grammar assertions. |

The target layout (including not-yet-created packages such as `webapis/`,
`engines/`, `farbling/`, and `javascripts/sobek/`) is described in
[`architecture.md`](./architecture.md). Those are design targets; only the packages in the
table above exist today.

## 5. Known limitations and partial work

These are intentional, tracked gaps rather than surprises:

- **Phase 4 is complete but deliberately scoped.** All Phase 4 tasks are `Verified`.
  Remaining CSS gaps are feature-depth gaps rather than task gaps: the CSSOM covers the
  core object model but not the full IDL (constructable/adopted stylesheets, every rule
  subclass), media queries do not implement serialization or every experimental feature,
  and cascade `revert-layer` follows the common origin/layer cases rather than every
  edge.
- **Computed values cover the style engine's supported property set.** Font, color,
  line-height, opacity, border widths, and generic length/angle/time conversion are
  implemented. Layout-specific used values are handled by `layouting`; percentage
  resolution against containing blocks, intrinsic sizing, and text metrics are present
  but several layout refinements remain partial.
- **Layout is fully verified against the geometry harness and paint integration.**
  Sticky positioning is applied during paint with scroll-container offsets,
  scroll containers paint track/thumb scrollbars, and text uses the real
  font/metrics/shaper path. Full WPT reftests remain unvendored; the 35-case
  geometry harness plus sticky/scrollbar paint tests are the current baseline.
- **Incremental restyle is conservative.** Dirty nodes restyle their parent subtree;
  stylesheets using `:has()` trigger a full restyle because descendant changes can
  invalidate ancestors. Selector indexes are rebuilt on restyle.
- **CSS parsing conformance corpus.** The public `css-parsing-tests` corpora target a
  pre-2021 tokenizer (match tokens such as `~=`, a `unicode-range` token, and normalized
  number representations). `cmd/corpusgen` normalizes the expected values for the pinned
  syntax; two non-ASCII identifier cases remain documented skips. The tokenizer, An+B,
  `<urange>`, and Color 4 corpora are vendored and pass.
- **Property registry and value skips.** The registry is generated for 353 properties;
  the stable value corpus excludes experimental/tentative properties and records 260
  skips (Color 5, experimental sizing/overflow, tentative grid-lanes, and deferred deep
  shorthand/image-function validation). The generator warns and skips four malformed
  spec grammar tables (`columns`, `inset-block-start`, `margin-block-start`,
  `padding-block-start`).
- **Color 4 lab/lch/oklab/oklch/color()** parse and serialize in their declared space, but
  gamut mapping and full reference serialization are not implemented or conformance-tested.
- **CSS identifier definitions.** The pinned `specifications/css/syntax-3.html` is the 2021
  Candidate Recommendation, which predates the current "non-ASCII ident code point" ranges.
  The tokenizer follows the current CSS Syntax 3 definition (matching WPT and browsers); the
  pinned spec should be refreshed with `specifications/update.sh`.
- **Painting still has feature gaps.** Fonts use embedded Go faces, system
  TrueType/OpenType files, and author `@font-face` faces (raw TTF/OTF and WOFF1;
  WOFF2 is rejected until a Brotli decoder is added). Partitioned font caches
  and `font-display` behavior are implemented; application of
  feature/variation settings and full GSUB/GPOS shaping remain blocked on
  P10-TEXT-001.
  Transforms cover 2D and 3D/projective translate/scale/rotate/skew/matrix,
  individual transform properties, and perspective; `transform-style:
  preserve-3d`, backface visibility, and filters remain. Replaced content
  supports `img`/`srcset`/`picture`, video posters, SVG, and object-fit;
  canvas requires scripting. Backgrounds support colors, decoded images, gradients
  (including sizing/position/repeat and fixed attachment), border images,
  radii, and shadows; `background-blend-mode` and gradient interpolation spaces
  (`in oklab`) are not implemented. SVG covers a static subset (no text,
  use/clipPath/mask/pattern/filter, or external resources). GIF/WebP animation
  metadata is decoded but only the first frame is painted. The WPT reftest
  corpus is not vendored; the local reftest comparator and golden corpus are the
  current baseline.
- **Color conversion uses a chroma-clamp gamut map.** All CSS Color 4 spaces
  convert to sRGB; out-of-gamut values reduce OKLCH chroma to fit. The CSS
  perceptual/JND gamut-mapping algorithm and WPT conversion vectors are not yet
  implemented.
- **The X11 backend is complete for X11 but not Wayland.** It handles input,
  clipboard, MIT-SHM, resize, and WM_DELETE; a Wayland backend is Phase 10. The
  integration tests are skipped when `DISPLAY` is unset.
- **Phase 8 browsing contexts are static and single-process.** Inline iframes
  load, render, and honor sandbox navigation flags, but scripts, `window.open`,
  cross-process iframe isolation, and the full HTML session-history API are out
  of scope until Phase 10. The privacy dashboard reports the broker's current
  cookie/cache/graph state; it is not a general network inspector.
- **Generated content is partially implemented.** `::before`/`::after` selectors
  are matched against their originating element and generate styled boxes with
  string content, `attr()` values, and open/close quotes, including empty-content
  boxes used for icons and clearing. Inline generated boxes are treated as
  atomic inline fragments so margins, padding, borders, backgrounds, and widths
  affect positioning like they do for inline-block content. CSS counters, `content: url()`, nested quote
  tracking, and non-`before`/`after` pseudo-element boxes are not implemented.
  Combined with the absence of scripting, this can still cause differences on
  real sites such as `cookie.engineer`.
- **P2-TLS-002 (ECH/GREASE)** — GREASE values and the ECH config field are present, but
  `crypto/tls` cannot inject GREASE or permute the ClientHello. That needs the planned
  `protocols/tls/chrome` fork.
- **P1-MICRO-003 (colors)** — the legacy color parser recognizes the basic HTML color set;
  the full CSS named-color table is deferred to the CSS color parser.
- **`x/net/html` namespaces** — the upstream parser uses short namespace names (`""`,
  `svg`, `math`); `parsers/html` maps these to the DOM namespace URLs and back.
- **No `sobek` fork yet.** The JavaScript engine is a Phase 10 deliverable. When vendored,
  its non-essential third-party dependencies (`sourcemap`, `pprof`, `yaml`, and possibly
  `semver`) must be pruned and `regexp2` replaced per the project decision.
- **No in-tree forks exist yet**, so no `FORK.md` files are present.
- **Specifications tree is ~50 MB** and committed for offline verification. See
  [`specifications.md`](./specifications.md) §5 for the LFS option.

## 6. Decisions that are already locked

Recorded in the relevant design docs under "Decisions":

- Module path `gonetsurf`; Go 1.27; stdlib + `golang.org/x/*` only.
- DOM package is `dom` (singular); category packages are plural.
- The WHATWG URL parser in `urls` is the only URL parser used for security/cache keys.
- No single-letter identifiers; descriptive names; comments only where they add value.
- Aggressive archive caching with a user-visible archive banner
  ([`cache.md`](./cache.md)).
- Persistent, partitioned cookie jar with a cross-site dependency graph and atomic
  tracking-session reset that defeats CNAME-cloaked trackers
  ([`privacy.md`](./privacy.md)).
- OS process per site/tab with a trusted network broker, sandboxed with
  namespaces/seccomp/Landlock/rlimits/cgroups ([`security.md`](./security.md)).
- Farbling is deterministic per origin+session via an HMAC of an install seed.

## 7. Where to look

| Question | Document |
|---|---|
| What are we building and in what order? | [`implementation-plan.md`](./implementation-plan.md) |
| What is done / tested / blocked? | [`task-tracking.md`](./task-tracking.md) |
| How is the engine structured? | [`architecture.md`](./architecture.md) |
| How does the cache map URLs to disk? | [`cache.md`](./cache.md) |
| How is privacy/tracking handled? | [`privacy.md`](./privacy.md) |
| How is untrusted content isolated? | [`security.md`](./security.md) |
| Coding and test conventions | [`conventions.md`](./conventions.md) |
| Which specs are pinned and how to refresh | [`specifications.md`](./specifications.md) |

## 8. Recommended next steps

1. **Resolve the remaining Phase 7 partial dependency.** Add the OpenType
   GSUB/GPOS shaper (P10-TEXT-001) for `font-feature-settings`,
   `font-variation-settings`, and variable-axis selection (P7-FONT-004).
2. **Vendor and run WPT reftests.** `netsurfctl reftest` is ready; add the WPT
   reftest subset and CI thresholds, then close the remaining non-blocking
   gaps (`background-blend-mode`, SVG text/use/clipPath/mask/pattern/filter,
   animated image playback).
3. **Start Phase 10.** Implement the pluggable/activatable JavaScript engine
   interface (sobek vendoring is a dependency decision), event/task scheduling,
   farbled Web API bindings, CSS animation runtime, paged media, Wayland, HTTP/2,
   and OpenType shaping. The renderer can also be split into its own supervised
   process using the completed IPC/shared-memory/broker stack.
4. **Deepen CSS feature coverage** as later phases need it: full CSSOM IDL
   surface, media query serialization, and cascade edge cases.
5. **Close the remaining Partial network task** (TLS GREASE/ECH) when the
   `protocols/tls/chrome` fork lands.
6. **Fill the skeleton.** Create remaining package directories as their phases
   begin; avoid broad empty scaffolding ahead of need.

## 9. Handover checklist

- [x] Builds, vets, formats, and tests clean.
- [x] Task ledger reflects reality and has no stale "no code yet" claims.
- [x] Design docs describe both the target architecture and the current state.
- [x] Known limitations are listed with their task IDs.
- [x] No secrets, credentials, or generated artifacts are committed.
- [ ] Everything under `docs/` reviewed by the next maintainer.
