# Plan: Agents Page

## Overview
New `/agents` page that replaces the agent-selection UX from the current permissions page. The existing `/permissions` page stays untouched. The agents page shows every stored agent as a row with rich detail (name, system, credits, ships, faction, construction status) with interactive filtering.

## Data Available (no new collection needed)

| Source | Fields |
|--------|--------|
| `GetAgents(reset)` | Symbol, Credits, Faction, Headquarters, System |
| `GetAgentRecordsShips(reset, agent, dur)` | Ship count history (latest point = current ships) |
| `GetJumpgates(reset)` → map by system | Jumpgate status per system (NoActivity/Active/Const/Complete) |
| `GetLatestConstructionRecords(reset, agents)` | Per-agent construction Fabmat/Advcct/Timestamp |
| `GetFactions(reset)` | Symbol, Name, Traits, Color (hardcoded map below) |

We need one new helper: `GetLatestShipsForAgents` that returns a `map[string]int64` of agent → latest ship count, derived from `GetAgentHistory`. This avoids calling `GetAgentRecordsShips` per-agent.

## Faction Colors (hardcoded in frontend)

| Symbol | Color | Name |
|--------|-------|------|
| COSMIC | #7B68EE | Cosmic Engineers |
| GALACTIC | #4169E1 | Galactic Alliance |
| QUANTUM | #00CED1 | Quantum Federation |
| DOMINION | #DC143C | Stellar Dominion |
| ASTRO | #DAA520 | Astro-Salvage Alliance |
| CORSAIRS | #8B0000 | Seventh Space Corsairs |
| VOID | #708090 | Voidfarers |
| OBSIDIAN | #2F2F2F | Obsidian Syndicate |
| AEGIS | #4682B4 | Aegis Collective |
| UNITED | #228B22 | United Independent Settlements |

## New Backend Work

### `internal/datastore/agent.go` — add `GetLatestShipsForAgents`

Returns `map[string]int64` (agent symbol → latest ship count) by loading `GetAgentHistory` once and picking the most recent entry per agent. This is more efficient than N calls to `GetAgentRecordsShips`.

### `internal/frontend/handlers.go` — add `AgentsHandler`

New handler that assembles an `AgentRow` struct per agent:

```go
type AgentRow struct {
    Symbol       string
    Factions      string
    Credits      int64
    Ships        int64
    System       string
    IsActive     bool   // credits != 175000
    FactionColor string
    FactionName  string
    Construction string  // human-readable: "Active 350/1600 FAB, 120/400 ADV" or "Complete" or "—"
    SystemCount  int    // how many agents share this system (for orange light)
}
```

Data Assembly:
1. `GetAgents(latestReset)` → base agent info
2. `GetLatestShipsForAgents(latestReset)` → ship counts
3. `GetJumpgates(latestReset)` → construction status per system
4. `GetLatestConstructionRecords(latestReset, allAgentNames)` → per-agent fabmat/advcct
5. `GetFactions(latestReset)` → build symbol→{name,color} map
6. Count agents per system for the "multiple agents" indicator

Query params: `agentSearch`, `hideInactive`, `sortBy` (name/credits), `faction` (filter), `system` (filter), `showConstruction` (filter: only building/built jumpgates).

### `internal/frontend/frontend.go` — register route

Add `http.HandleFunc("/agents", AgentsHandler)`.

## Template: `internal/frontend/templates/agents.html`

Layout follows existing permissions-grid pattern but as a table with interactive features:

```
┌─────────────────────────────────────────────────────────────┐
│ Agents                                                       │
│ [Search...] [Hide Inactive ☑] [Sort: Name▼]               │
│ [Faction: All ▼] [Only Construction ☐] [System: All ▼]     │
│                                                              │
│ ┌──────────────────────────────────────────────────────────┐ │
│ │ 🟢 AGENT_NAME  | Faction | System 🟡 | Ships | Credits │ │
│ │     Construction: Active 350/1600 FAB 120/400 ADV       │ │
│ └──────────────────────────────────────────────────────────┘ │
│ ...                                                          │
└─────────────────────────────────────────────────────────────┘
```

### Row structure
- Agent name: button with green (selected) / red (not selected) indicator light — toggles chart inclusion (writes to localStorage, same key as permissions page)
- Faction: colored pill button, click filters to that faction only
- System: text with orange dot if >1 agent in that system, click filters to that system
- Ships: number
- Credits: number
- Construction status: either `"350/1600 FB, 120/400 AC"` or `"Complete"` or `"—"` (no construction)

### Mobile responsiveness
- On screens <768px, each agent row collapses from a single horizontal row to a stacked card:
  - Line 1: Agent button + faction pill
  - Line 2: System + Ships
  - Line 3: Credits
  - Line 4: Construction status
- Uses CSS `@media (max-width: 768px)` with flex-wrap

## CSS additions in `internal/frontend/static/style.css`

- `.agents-container` — wrapper matching `.preferences-container`
- `.agents-table` — table with responsive override
- `.agent-row` — flex row that wraps on mobile
- `.faction-pill` — small colored round button
- `.system-indicator` — orange dot (`.system-multi`) when >1 agent in system
- `.construction-status` — monospaced detail line
- Mobile breakpoint collapses rows to vertical cards

## Nav update: `internal/frontend/templates/nav.html`

Add a new entry:
```html
<li><a href="#" hx-get="/agents" hx-target="#content-area" class="nav-link">Agents</a></li>
```

Placed above the charts section.

## Files to Change

| File | Change |
|------|--------|
| `internal/datastore/agent.go` | Add `GetLatestShipsForAgents(reset) map[string]int64` |
| `internal/frontend/handlers.go` | Add `AgentsHandler` with `AgentRow` struct |
| `internal/frontend/frontend.go` | Register `/agents` route |
| `internal/frontend/templates/agents.html` | New template |
| `internal/frontend/templates/nav.html` | Add Agents nav link |
| `internal/frontend/static/style.css` | Add agents page styles with mobile breakpoints |
