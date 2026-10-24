# Supply Chain Tree Page Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a "Supply Chain" page that renders a chosen good's recursive ingredient chain as an ECharts right-to-left tree (phase 1: "what do I need to produce…"), with the inverse "what can I produce with…" left-to-right tree as phase 2.

**Architecture:** One new route (`/supply-chain`) + handler + template + nav entry. The handler loads the whole export→imports map once per request via the existing `ds.GetSupplyChain()` and hands it to pure transforms in `charts.go`: a tree walker with two stop rules (EXPLOSIVES is never expanded; a good already on the current path is not re-expanded), a per-mode sorted goods list, and a `charts.Tree` builder rendered via `RenderSnippet()` into the fragment, exactly like the jumpgate page. No new dependencies, no DB/collector changes.

**Tech Stack:** stdlib + `html/template` + htmx fragments + `go-echarts/v2` v2.7.1 (`charts.NewTree`, `opts.TreeData`, `opts.TreeChart` via `charts.WithTreeOpts`, `opts.Bool`, `RenderSnippet()` — all verified present in the vendored version; `opts.TreeData.Children` is `[]*TreeData`).

**Spec:** `docs/superpowers/specs/2026-10-24-supply-chain-tree-design.md`

**Note on the DB call:** the spec's per-node query (`SELECT import FROM supply_chain WHERE export = X`) is satisfied by the existing `GetSupplyChain()` — one read of the whole static table per request, then the recursion walks the in-memory map (`chain[node]`). This matches the house pattern (handler reads datastore once → pure transform) and avoids a new per-node getter; the table is static and a few dozen rows, so the map is small either way. Flag to reviewer: if you prefer a dedicated SQL getter, say so at review.

## Global Constraints

- **No new external dependencies**; no schema, collector, or datastore changes — the only data call is the existing `GetSupplyChain()`.
- Scope cap: the page uses only the already-collected `GET /market/supply-chain` data. No market/price/good-detail endpoints, no per-node SQL.
- Transforms are pure functions in `charts.go` (never touch the filesystem); handlers nil intermediates for GC per `AGENTS.md`.
- ECharts JS is already loaded globally in `index.html`; charts are injected as `RenderSnippet()` `template.HTML` fragments only.
- Dev shell gotcha: direnv exports `FLUFFY_*_TICK` vars that break `internal/collector` tick tests, and the sandbox `GOCACHE` is read-only. Run tests as:
  `FLUFFY_AGENT_TICK= FLUFFY_JUMPGATE_TICK= FLUFFY_CONSTRUCTION_TICK= FLUFFY_SUPPLY_TICK= GOCACHE=/tmp/fluffy-gocache go test ./...`
- Before merge: `go build ./...`, `go vet ./...`, `gofmt -l .` (empty), tests with `-count=1`, `go test -race ./...` all clean.

## Review Focus

1. **Cyclic chain data** (A→B→A in the map): must terminate; the repeated good appears as an unexpanded leaf. Pinned by `TestBuildNeedsTree_CycleGuard` (Task 1).
2. **EXPLOSIVES noise rule**: never expanded, in both modes and as root. Pinned by `TestBuildNeedsTree_ExplosivesLeaf` (Task 1) and `TestBuildProductsTree_ExplosivesLeaf` (Task 4).
3. **Fresh boot, chain not yet collected** (`GetSupplyChain()` empty): page shows the dropdowns + "not collected yet" message, never an empty/broken chart. Template branch, exercised manually in Task 3 (no handler unit tests in this codebase).
4. **Stale/unknown `good` or `mode` query params**: unknown good → single-node tree (no error); invalid/absent mode → "needs". Pinned by `TestBuildNeedsTree_UnknownGood` (Task 1) + handler default checked in Task 3's manual pass.
5. **Deep/wide trees** (e.g. ADVANCED_CIRCUITRY): everything expanded and pannable — `initialTreeDepth:-1` + `roam:true` pinned by `TestSupplyChainChart_SnippetPins` (Task 2), eyeballed in the browser (Task 3).

---

### Task 1: Needs-mode tree transforms (pure functions)

**Files:**
- Modify: `internal/frontend/charts.go` (append)
- Test: `internal/frontend/supplychain_test.go` (new, `package frontend`)

**Interfaces:**
- Consumes: nothing (pure).
- Produces (all in `charts.go`; later tasks rely on these exact names):
  - `const explosivesGood = "EXPLOSIVES"`
  - `type chainNode struct { Name string; Children []*chainNode }`
  - `func buildChainTree(adj map[string][]string, good string) *chainNode` — generic walker (children = `adj[node]`, stop rules below)
  - `func buildNeedsTree(chain map[string][]string, good string) *chainNode` — `buildChainTree(chain, good)`
  - `func chainGoods(chain map[string][]string, mode string) []string` — `"needs"` (and any other value, as default): sorted export keys; the `"products"` branch is added in Task 4; empty chain ⇒ empty non-nil slice
  - `func buildTreeData(n *chainNode) opts.TreeData` — recursive conversion (`Children []*opts.TreeData`)

- [ ] **Step 1: Write the failing tests**

In `supplychain_test.go`, with this fixture:

```go
func testChain() map[string][]string {
	return map[string][]string{
		"FAB_MATS":   {"IRON", "QUARTZ_SAND"},
		"IRON":       {"IRON_ORE"},
		"SHIP_PARTS": {"EXPLOSIVES", "IRON"},
		"EXPLOSIVES": {"AMMONIA_ICE"},
	}
}
```

- `TestBuildNeedsTree_FabMats` (the spec's example): `buildNeedsTree(testChain(), "FAB_MATS")` → root `FAB_MATS`; children named `IRON`, `QUARTZ_SAND` in that order; `IRON` has exactly one child `IRON_ORE` with no children; `QUARTZ_SAND` is a leaf.
- `TestBuildNeedsTree_ExplosivesLeaf`: root `SHIP_PARTS` → children `EXPLOSIVES`, `IRON`; the `EXPLOSIVES` child has **no** children (AMMONIA_ICE hidden); and root `EXPLOSIVES` → single node, no children.
- `TestBuildNeedsTree_CycleGuard`: `{"A": {"B"}, "B": {"A"}}` → `A` → `B` → `A`-leaf; walk terminates; the third node has no children.
- `TestBuildNeedsTree_UnknownGood`: `buildNeedsTree(testChain(), "WATER_ICE")` → single node `WATER_ICE`, no children, no panic.
- `TestBuildNeedsTree_EmptyChain`: empty map → single node.
- `TestChainGoods_Needs`: `chainGoods(testChain(), "needs")` = `["EXPLOSIVES","FAB_MATS","IRON","SHIP_PARTS"]`; `chainGoods(testChain(), "bogus")` returns the same (default); `chainGoods(map[string][]string{}, "needs")` is empty non-nil.
- `TestBuildTreeData`: `buildTreeData(buildNeedsTree(testChain(), "FAB_MATS"))` → `.Name == "FAB_MATS"`, `len(.Children) == 2`, `.Children[0].Children[0].Name == "IRON_ORE"`.

- [ ] **Step 2: Run to verify failure**

Run: `FLUFFY_AGENT_TICK= FLUFFY_JUMPGATE_TICK= FLUFFY_CONSTRUCTION_TICK= FLUFFY_SUPPLY_TICK= GOCACHE=/tmp/fluffy-gocache go test ./internal/frontend/ -run 'TestBuildNeeds|TestChainGoods|TestBuildTreeData' -v`
Expected: FAIL — `undefined: buildNeedsTree` etc.

- [ ] **Step 3: Implement in `charts.go`**

Public wrappers are one-liners; the walker body (the only algorithm the tests don't determine):

```go
func buildChainNode(adj map[string][]string, good string, onPath map[string]bool) *chainNode {
	node := &chainNode{Name: good}
	if good == explosivesGood || onPath[good] {
		return node
	}
	onPath[good] = true
	for _, dep := range adj[good] {
		node.Children = append(node.Children, buildChainNode(adj, dep, onPath))
	}
	delete(onPath, good) // DFS backtrack: siblings may repeat a good in their own branches
	return node
}

func buildChainTree(adj map[string][]string, good string) *chainNode {
	return buildChainNode(adj, good, map[string]bool{})
}
```

`buildNeedsTree` = `buildChainTree(chain, good)`. `chainGoods` collects export keys into a set, returns the sorted slice (default branch = "needs"). `buildTreeData` recurses into `opts.TreeData{Name, Children []*opts.TreeData}`.

- [ ] **Step 4: Run to verify pass**

Run: same `-run` as Step 2. Expected: PASS. Then `gofmt -l internal/frontend/` (empty) and `go vet ./internal/frontend/`.

- [ ] **Step 5: Commit**

```
git add internal/frontend/charts.go internal/frontend/supplychain_test.go
git commit -m "feat(frontend): supply chain needs-mode tree transforms"
```

### Task 2: `SupplyChainChart` ECharts tree builder

**Files:**
- Modify: `internal/frontend/charts.go`
- Test: `internal/frontend/supplychain_test.go`

**Interfaces:**
- Consumes: Task 1 (`chainNode`, `buildTreeData`).
- Produces: `func SupplyChainChart(root *chainNode, title, orient string) *charts.Tree` — `orient` is `"RL"` (phase 1) or `"LR"` (phase 2); dark theme, 100% width, height `750` like the jumpgate parallel chart.

- [ ] **Step 1: Write the failing test**

```go
func TestSupplyChainChart_SnippetPins(t *testing.T) {
	root := buildNeedsTree(testChain(), "FAB_MATS")
	snippet := SupplyChainChart(root, "Requirements for FAB_MATS", "RL").RenderSnippet()
	for _, want := range []string{`"orient":"RL"`, `"roam":true`, `"initialTreeDepth":-1`, "IRON_ORE"} {
		if !strings.Contains(snippet.Script, want) {
			t.Errorf("snippet missing %q", want)
		}
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `... go test ./internal/frontend/ -run TestSupplyChainChart -v` (same env prefix as Task 1)
Expected: FAIL — `undefined: SupplyChainChart`.

- [ ] **Step 3: Implement in `charts.go`**

```go
func SupplyChainChart(root *chainNode, title, orient string) *charts.Tree {
	tree := charts.NewTree()
	tree.SetGlobalOptions(
		charts.WithInitializationOpts(opts.Initialization{Theme: "dark", Width: "100%", Height: "750"}),
		charts.WithTitleOpts(opts.Title{Title: title}),
		charts.WithTooltipOpts(opts.Tooltip{Show: opts.Bool(true), Trigger: "item"}),
	)
	tree.AddSeries(root.Name, []opts.TreeData{buildTreeData(root)},
		charts.WithTreeOpts(opts.TreeChart{
			Layout:          "orthogonal",
			Orient:          orient,
			Roam:            opts.Bool(true), // pan/zoom for deep chains
			InitialTreeDepth: -1,             // fully expanded
		}),
	)
	return tree
}
```

- [ ] **Step 4: Run to verify pass**

Run: same `-run`; PASS. `gofmt -l internal/frontend/` empty; `go vet ./internal/frontend/` clean.

- [ ] **Step 5: Commit**

```
git add internal/frontend/charts.go internal/frontend/supplychain_test.go
git commit -m "feat(frontend): SupplyChainChart — echarts RL/LR tree builder"
```

### Task 3: Page — handler, template, route, nav entry, docs

**Files:**
- Modify: `internal/frontend/handlers.go` (new handler)
- Modify: `internal/frontend/frontend.go` (route)
- Create: `internal/frontend/templates/supplychain.html`
- Modify: `internal/frontend/templates/nav.html`
- Modify: `docs/INTERNAL.md` (routes table)

**Interfaces:**
- Consumes: Task 1–2 functions, `ds.GetSupplyChain()`, `ChartSnippet`, `t.ExecuteTemplate`, `metrics.RecordDuration`.
- Produces: route `GET /supply-chain` (`?mode=needs&good=SYMBOL`), `func SupplyChainHandler(w http.ResponseWriter, r *http.Request)`.

No unit tests for the handler/template — this codebase tests pure transforms only; correctness is pinned by Task 1–2 tests plus the manual pass below.

- [ ] **Step 1: Add the handler in `handlers.go`** (after `FactionsHandler`)

```go
func SupplyChainHandler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	q := r.URL.Query()
	mode := q.Get("mode")
	if mode != "needs" {
		mode = "needs" // Task 4 adds "products"
	}
	good := q.Get("good")

	chain, err := ds.GetSupplyChain()
	if err != nil {
		logging.Error("error loading supply chain", err)
		chain = map[string][]string{}
	}

	data := struct {
		Mode      string
		Good      string
		Goods     []string
		Chart     ChartSnippet
		Collected bool
	}{Mode: mode, Good: good, Goods: chainGoods(chain, mode), Collected: len(chain) > 0}

	if good != "" && len(chain) > 0 {
		root := buildNeedsTree(chain, good)
		snippet := SupplyChainChart(root, "Requirements for "+good, "RL").RenderSnippet()
		data.Chart = ChartSnippet{
			Element: template.HTML(snippet.Element),
			Script:  template.HTML(snippet.Script),
		}
	}

	chain = nil // GC convention

	w.Header().Set("Content-Type", "text/html")
	if err := t.ExecuteTemplate(w, "supplychain.html", data); err != nil {
		logging.Error("template error", err)
	}
	metrics.RecordDuration("supply_chain", start)
}
```

- [ ] **Step 2: Create `templates/supplychain.html`**

```html
<h2>Supply Chain</h2>

<div class="controls-container">
    <label class="select-label">Mode:
        <select name="mode"
                hx-get="/supply-chain"
                hx-target="#content-area"
                hx-include="[name='good']">
            <option value="needs"{{if eq .Mode "needs"}} selected{{end}}>What do I need to produce…</option>
        </select>
    </label>
    <label class="select-label">Good:
        <select name="good"
                hx-get="/supply-chain"
                hx-target="#content-area"
                hx-include="[name='mode']">
            <option value="">Select a good…</option>
            {{$g := .Good}}{{range .Goods}}<option value="{{.}}"{{if eq . $g}} selected{{end}}>{{.}}</option>{{end}}
        </select>
    </label>
</div>

{{if not .Collected}}
<p>Supply chain data has not been collected yet; it appears after the next collector tick.</p>
{{else if .Chart.Element}}
<div class="chart-scroll-wrapper">
  <div>{{ .Chart.Element }} {{ .Chart.Script }}</div>
</div>
{{else}}
<p>Select a good to see its supply chain.</p>
{{end}}
```

- [ ] **Step 3: Route in `frontend.go` + nav entry in `nav.html`**

`frontend.go`, after the `/factions` line: `http.HandleFunc("/supply-chain", SupplyChainHandler)`

`nav.html`, after the Factions `</li>` (before the `nav-section` Charts divider):

```html
            <li><a href="#" hx-get="/supply-chain" hx-target="#content-area" class="nav-link" data-tooltip="Supply Chain">
                <span class="nav-icon">
                    <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/></svg>
                </span>
                <span class="nav-label">Supply Chain</span>
            </a></li>
```

(feather-icons "link" — two interlocking chain links; matches the existing nav SVG style)

- [ ] **Step 4: Docs**

`docs/INTERNAL.md` routes table (after the `/factions` row):

`| /supply-chain | SupplyChainHandler | Supply chain tree (?mode=needs, ?good=SYMBOL) |`

- [ ] **Step 5: Verify**

Run: `go build ./...`, `go vet ./...`, `gofmt -l .` (empty), and (env prefix per Global Constraints) `go test ./... -count=1` then `go test -race ./...` — all clean.

Manual pass (loopback shell or browser): seed a scratch DB and click/curl through —

```bash
FLUFFY_PORT=:8891 FLUFFY_STORAGE_PATH=/tmp/fluffy-sc go run main.go &
sqlite3 /tmp/fluffy-sc/fluffy.db "INSERT INTO supply_chain (export, import, fetched_at) VALUES
  ('FAB_MATS','IRON',0),('FAB_MATS','QUARTZ_SAND',0),('IRON','IRON_ORE',0)"
curl -s 'localhost:8891/supply-chain?good=FAB_MATS' | grep -c IRON_ORE   # ≥1: tree rendered
curl -s 'localhost:8891/supply-chain' | grep -c 'Select a good'          # 1: placeholder state
```

Check in a browser: nav icon + dropdowns render; FAB_MATS shows the right-to-left tree from the spec (FAB_MATS rightmost, IRON + QUARTZ_SAND left of it, IRON_ORE leftmost); `/supply-chain?mode=bogus` falls back to needs; an unseeded DB shows the "not collected yet" message.

- [ ] **Step 6: Commit**

```
git add internal/frontend/handlers.go internal/frontend/frontend.go internal/frontend/templates/supplychain.html internal/frontend/templates/nav.html docs/INTERNAL.md
git commit -m "feat(frontend): /supply-chain page — needs-mode RL tree + nav entry + docs"
```

### Task 4 (Phase 2): Products mode — reverse map, LR tree, second dropdown option

**Files:**
- Modify: `internal/frontend/charts.go`
- Modify: `internal/frontend/handlers.go`
- Modify: `internal/frontend/templates/supplychain.html`
- Modify: `docs/INTERNAL.md` (routes row: `?mode=needs|products`)
- Test: `internal/frontend/supplychain_test.go`

**Interfaces:**
- Consumes: Tasks 1–3.
- Produces:
  - `func reverseChain(chain map[string][]string) map[string][]string` — import → sorted exports
  - `func buildProductsTree(chain map[string][]string, good string) *chainNode` — `buildChainTree(reverseChain(chain), good)`
  - `chainGoods` gains the `"products"` branch: sorted distinct import symbols
  - Handler accepts `mode=products`: LR orient, title `"Products from "+good`

- [ ] **Step 1: Write the failing tests**

- `TestReverseChain`: `reverseChain(testChain())` = `{"IRON": ["FAB_MATS","SHIP_PARTS"], "IRON_ORE": ["IRON"], "QUARTZ_SAND": ["FAB_MATS"], "AMMONIA_ICE": ["EXPLOSIVES"]}` (each list sorted).
- `TestBuildProductsTree_Iron`: root `IRON` → children `["FAB_MATS","SHIP_PARTS"]` (sorted), both leaves in the reversed graph.
- `TestBuildProductsTree_ExplosivesLeaf`: chain `{"MISSILE": {"EXPLOSIVES"}, "EXPLOSIVES": {"FUEL"}, "FUEL": {"OIL"}}`, root `OIL` → `FUEL` → `EXPLOSIVES` leaf (MISSILE hidden); root `EXPLOSIVES` → single node.
- `TestChainGoods_Products`: `chainGoods(testChain(), "products")` = `["AMMONIA_ICE","EXPLOSIVES","IRON","IRON_ORE","QUARTZ_SAND"]`.
- `TestSupplyChainChart_LRSnippet`: `SupplyChainChart(root, "Products from IRON", "LR")` snippet contains `"orient":"LR"`.

- [ ] **Step 2: Run to verify failure**

Run: `... go test ./internal/frontend/ -run 'TestReverseChain|TestBuildProductsTree|TestChainGoods_Products|TestSupplyChainChart_LRSnippet' -v`
Expected: FAIL — `undefined: reverseChain` etc.

- [ ] **Step 3: Implement**

- `charts.go`: `reverseChain` inverts the map, sorting each export list; `buildProductsTree` per Interfaces; `chainGoods` `"products"` branch collects distinct imports.
- `handlers.go`: mode check becomes `if mode != "needs" && mode != "products" { mode = "needs" }`; the chart block branches:

```go
	if good != "" && len(chain) > 0 {
		orient, title := "RL", "Requirements for "+good
		root := buildNeedsTree(chain, good)
		if mode == "products" {
			root = buildProductsTree(chain, good)
			orient, title = "LR", "Products from "+good
		}
		snippet := SupplyChainChart(root, title, orient).RenderSnippet()
		// ... unchanged ChartSnippet assignment
	}
```

- `supplychain.html`: add after the `needs` option:
  `<option value="products"{{if eq .Mode "products"}} selected{{end}}>What can I produce with…</option>`
- `docs/INTERNAL.md`: routes row → `(?mode=needs|products, ?good=SYMBOL)`.

- [ ] **Step 4: Verify**

Same full gate as Task 3 Step 5, plus manual: mode swap keeps the selected good (or safely drops to placeholder if the good is not valid for the new mode's dropdown); IRON in products mode shows the left-to-right tree; EXPLOSIVES is a leaf in both modes.

- [ ] **Step 5: Commit**

```
git add internal/frontend/ docs/INTERNAL.md
git commit -m "feat(frontend): supply chain products mode — LR tree"
```

---

## Self-Review Notes

- Spec coverage: nav entry + icon (T3), full-page tree + two dropdowns (T3, second mode option T4), RL tree + FAB_MATS example (T1–3), LR tree (T4), EXPLOSIVES rule both modes (T1/T4 tests), cycle guard (T1), unknown good / empty store handling (T1 + T3 template), echarts usage per jumpgate page pattern (T2–3).
- Name consistency: `chainNode`, `buildChainTree`, `buildNeedsTree`, `buildProductsTree`, `reverseChain`, `chainGoods`, `buildTreeData`, `SupplyChainChart(root, title, orient)`, route `/supply-chain`, mode values `needs`/`products`, metric `supply_chain` — identical across tasks.
- Proportion: 4 tasks, 2 new files (`supplychain_test.go`, `supplychain.html`), 5 small edits; each task is independently green and commit-able; phase boundary = between Task 3 and Task 4 (pause for review after the needs-mode page is live).