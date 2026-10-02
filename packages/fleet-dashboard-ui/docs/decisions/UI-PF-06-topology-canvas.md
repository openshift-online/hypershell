# UI-PF-06 - Decision record: the topology canvas is the one sanctioned custom component

**Status:** Accepted
**Standard:** `patternfly.spec.md` UI-PF-06 (custom component requires a decision
record: gap evidenced, alternatives considered, public API, a11y contract,
retirement path).
**Context specs:** operational-dashboard `ui-architecture.spec.md` §6 (the one
sanctioned custom component), §3.5 (accessible equivalent), §7 (performance).

> This file lives in the public app repo and therefore names **no** fleet
> identity. Instances, clusters, namespaces, and hostnames are runtime data
> discovered from the BFF, never compiled in - see the component README's
> fleet-identity firewall section.

## 1. Gap evidenced

The product centerpiece is an **interactive pan / zoom / fling SVG promotion
graph**: environment lanes laid out as a DAG, promotion gates rendered as
funnels between lanes, per-instance node cards carrying a deterministic
identicon, a gateway-phase donut, a 24 h gateway-count "sand" sparkline, and a
release-bundle freight rail coupled to the graph by selection. A minimap tracks
the viewport; a details drawer opens on selection.

PatternFly 6 (React) ships no primitive that renders any of this:

- **PatternFly Topology** models a force/graph-style node-edge canvas, but it has
  no concept of ordered promotion **lanes**, gate **funnels** between lanes, or a
  **freight timeline** coupled to graph selection. Bending it to a fixed DAG-lane
  layout would mean fighting its layout engine and re-styling every primitive -
  more custom surface, not less, and a worse a11y story than purpose-built SVG.
- **PatternFly Charts / a charting library (d3, visx, nivo, …)** solve
  cartesian/statistical charts, not an interactive domain graph with bespoke
  nodes. Pulling one in adds a large dependency and a second layout/animation
  model for the _only_ thing it would serve (the inline sparkline), which is a
  dozen lines of SVG `<polyline>` from a pure domain function.

The gap is real and specific; the layout + bundle-timeline coupling is the
differentiator that no off-the-shelf primitive provides.

## 2. Alternatives considered and rejected

| Alternative                             | Rejected because                                                                                         |
| --------------------------------------- | -------------------------------------------------------------------------------------------------------- |
| PatternFly Topology view                | No lane/funnel/freight model; retro-fitting is more custom surface and weaker a11y than hand-rolled SVG. |
| Charting library for the whole canvas   | Wrong abstraction (statistical charts ≠ interactive domain graph); heavyweight dependency.               |
| Charting library for just the sparkline | Not worth a dependency for a pure-function `<polyline>`; keeps the canvas on one rendering model.        |
| Plain DOM / absolutely-positioned divs  | Loses zoomable vector geometry, crisp edges, and a single `viewBox` transform for pan/zoom.              |

**Decision:** a hand-rolled, React-rendered SVG canvas. Everything _around_ it -
page chrome, masthead, toolbar buttons, the details drawer, the accessible
table, labels, badges, tooltips - stays stock PatternFly React. The custom
surface is confined to `src/adapters/presentation/map/`.

## 3. Architecture

- **Rendering is declarative and reactive.** React renders SVG from TanStack
  Query data; there is no imperative "redraw" path and no hand-built DOM. One
  query per plane, not per node (§7): node count does not grow the request count.
- **Layout + model are pure domain functions** (`src/domain/map/`,
  framework-free, unit-tested ≥ 80 % gate): `buildMapModel`, `computeLayout`,
  identicon, identiname, bundle derivation. No browser globals, no `Date.now` /
  `Math.random`, so output is deterministic and testable.
- **Pan / zoom / momentum** live in `useMapViewport`, a state-driven
  `requestAnimationFrame` loop that keeps the `viewBox` in React state (not
  DOM-mutated), easing the live view toward a target and decaying release
  velocity into a fling. This conforms to the React Compiler / React Hooks rules
  (no ref access during render; all mutable bookkeeping in refs touched only from
  handlers / the rAF callback).
- **Reduced motion:** `prefers-reduced-motion` disables easing and fling (instant
  snaps).

## 4. Public API

```ts
<TopologyMap promotion={PromotionData} fleet={FleetData} />
```

- Inputs are the already-decoded domain DTOs from the BFF; the component owns no
  fetching and no fleet identity.
- Internal seams (not a stability surface): `useMapViewport(contentWidth,
contentHeight) → { svgRef, viewBox, viewport, onWheel, onPointer*, zoomIn,
zoomOut, fit, didPan }`; `MapNodeCard`, `MapEdges`, `MiniMap`, `FreightBar`,
  `MapDetails`.
- All user-facing copy routes through the FormatJS catalog (`src/messages.ts`);
  node/gate/bundle text is server **data**, not catalog strings.

## 5. Accessibility contract (§3.5)

- The SVG canvas is an `application`/labelled region; nodes and gates are
  `role="button"` with `tabIndex={0}`, `aria-label`, `aria-pressed`, and keyboard
  activation (Enter/Space).
- A **non-visual accessible equivalent** ships alongside the canvas: the existing
  promotion table is retained (not replaced) as the programmatic representation
  of the same promotion state, so the view is fully usable without the graphic.
- The minimap is `aria-hidden` (decorative; the viewport is reachable via the
  canvas controls). Zoom/fit controls are real PatternFly `Button`s with
  catalog `aria-label`s.
- Target is the AA baseline (§10); automated a11y score alone is **not** proof
  (UI-VER-08) - behavior is exercised in tests.

## 6. Performance posture (§7)

- One query per plane; the canvas re-renders from state and relies on React
  Compiler auto-memoization. The sand sparkline history is a bounded array
  (≤ ~32 samples) computed server-side (stateless 24 h range query), so payload
  does not grow unbounded per instance.
- Must hold INP ≤ 200 ms at fleet scale; `touch-action: none` on the canvas keeps
  gesture handling off the main-thread scroll path.

## 7. Retirement path

If PatternFly Topology (or a future PatternFly primitive) grows lane/funnel/
freight-timeline support with an equivalent a11y contract, this component is
**retired in favour of it**: the pure domain layer (`src/domain/map/`) is
rendering-agnostic and is the only piece that must survive the swap; the
`src/adapters/presentation/map/` adapters are replaceable. Until then this is the
single justified custom component; no further custom components are sanctioned by
this record.
