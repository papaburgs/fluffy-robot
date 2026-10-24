# Supply Chain Tree Page — Design

Source: user request (2026-10-24 conversation). The backend (`supply_chain` table +
`updateSupplyChain` collector) is already merged; this spec covers the frontend page only.

## Page

- New nav menu entry **Supply Chain** with a chain-link icon (same feather-style inline SVG
  as the other nav icons), placed after Factions.
- Opens a full-page tree graph at `/supply-chain`, swapped into `#content-area` by htmx
  like every other page.
- Two dropdown boxes on the page:
  1. **Mode** — which direction the tree grows. Phase 1 ships one option; phase 2 adds the second.
  2. **Good** — the symbol to chart; the list depends on the mode.
- Data source: the existing `supply_chain` table via the existing `GetSupplyChain()`
  getter (export → imports map). No new SQL, no schema change, no collector change.

## Mode 1 (phase 1): "What do I need to produce…"

Right-to-left ECharts tree (`orient: "RL"`, as in the Apache
`tree-orient-right-left` example): the chosen good is the rightmost node; everything
needed to make it sits one column to the left; each of those expands the same way,
recursively.

Example — FAB_MATS (to export FAB_MATS you need iron and quartz sand; to get iron you
need iron ore):

```
    IRON_ORE  ----    IRON      -----
                                    |
                                     ----- FAB_MATS
                                    |
                   QUARTZ_SAND  ----
```

Semantics: the children of a node are `SELECT import FROM supply_chain WHERE export =
<node>` — i.e. `chain[node]` from the `GetSupplyChain()` map. The good dropdown for this
mode lists export symbols (things you can produce), sorted.

## Mode 2 (phase 2): "What can I produce with…"

The inverse function: left-to-right tree (`orient: "LR"`); the chosen good is the
leftmost node; to its right sits everything that can be produced from it, recursively.
Children of a node = exports whose import list contains the node (the reversed map).
The good dropdown for this mode lists import symbols (things you can feed in), sorted.

## Tree rules (both modes)

- **EXPLOSIVES is never expanded.** When the walk reaches EXPLOSIVES it is rendered as a
  leaf — its subtree introduces extra noise and is not shown. The rule applies in both
  modes, and to the root when EXPLOSIVES itself is selected (single-node tree).
- **Cycle guard.** A good that already appears on the path from the root to the current
  node is shown once more as a leaf but not expanded again — no infinite recursion, no
  duplicated endless branches. The same good may still appear in separate branches.
- A stale/unknown good (e.g. via hand-edited URL) renders as a single leaf node.
- Chain not collected yet (fresh boot before the first collector tick): the page renders
  the dropdowns plus an explanatory message instead of a chart.

## Chart

go-echarts v2 `charts.Tree` (the version already vendored): dark theme, 100% width,
item tooltip, roam (pan/zoom) enabled, all levels expanded. Rendered via
`RenderSnippet()` into the page fragment exactly like the jumpgate parallel chart.