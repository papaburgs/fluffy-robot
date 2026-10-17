# SQLite Datastore — Design

Date: 2026-10-17
Status: draft, awaiting approval
Tracking: forgejo issue #7 (umbrella); resolves #3, #4, #6; folds in the #5 fix.
Next step after approval: implementation plan (superpowers:writing-plans).

## Intent and why

The app runs as a single Railway container and must stay inside the $1/month free
plan. Today 43¢ of the 51¢ monthly bill is memory. Observed behavior on the pod:

- After a bounce the floor is ~18 MB; **one page refresh** pushes RSS to ~120 MB
  and it does not come back down.
- RSS then climbs ~20 MB/week until the pod is manually bounced again.

Root cause (verified by the bounce experiment above): every history request
(`/chart`, `/agents-grid`, `/jumpgates`) loads, decompresses, and gob-decodes the
**entire current reset week** (~2000 files by day 7) into RAM, then filters by
time range. The allocation peak grows daily, Go's runtime holds memory to cover
the peak and returns it slowly, so the floor ratchets up. On top of that, every
read/write creates a fresh zstd encoder/decoder, adding churn.

The fix is structural: replace the file-based datastore with an embedded SQLite
database so a request reads only the rows it needs (`WHERE timestamp BETWEEN …`),
and the allocation peak is flat regardless of how much data accumulates.

**Success criteria:**

1. Flat RAM at roughly the post-bounce floor (~50 MB), indefinitely — no more
   scheduled pod bounces.
2. Stay inside the Railway free plan (memory-dominant bill ≤ ~50¢).
3. No missed collection cycles: per-cycle data is complete and consistent.
4. Five full game weeks of history retained, pruned automatically at the
   Sunday reset boundary.

## Goals

- Embedded SQLite datastore replacing the gob.zst file store. Collector-facing
  `Store*` functions keep their shape; **frontend call sites change where flat RAM
  requires it** (agent-filtered history queries, aggregate "latest" getters) —
  the owner explicitly OK'd frontend changes as long as the data is consistent.
- History getters that query by time range **and requested agent set** (fixes
  the memory pressure, #3 — a `period=7d` chart must not load all agents' rows).
- Reset as a data column so cross-reset queries work (fixes #4).
- Factions actually collected — one fetch per reset — and stored (resolves #6 as
  "collect").
- Automatic retention pruning: **5 full game weeks** + the current partial week,
  pruned once per Sunday reset — no manual cleanup.
- Loud write failures: errors returned, logged, and counted in expvar.
- Tests for the new datastore package.

## Non-goals

- No UI redesign. Rendering may look slightly different; URL bookmarks may
  break. Data continuity is the requirement, not pixel-identical pages.
- No import of existing gob.zst data — fresh start (decided with owner).
- No hosted/remote database. A Turso/libSQL attempt was tried before
  (`libsql-client-go` in go.mod); usage costs were too high, and storage on
  Railway is cheap compared to network and RAM. The dependency gets deleted.
- No changes to collector schedules (agents 5 min, jumpgates 30 min,
  construction 4 h) or to the game-reset handling logic.

## Anti-goals (failure even if it "works")

- Any request path that allocates proportional to total stored data.
- Silently swallowed write errors (current `StoreAgents`/`StoreStats`/
  `StoreLeaderboards` ignore `writeData` errors — this is a bug we are
  explicitly fixing, not carrying over).
- A missed collection cycle (a partial or missing 5-minute window) — the single
  worst outcome for this project. Owner: "consistent data is the main
  requirement; everything else can be tweaked later."
- Any external service dependency for storage.

## Constraints

- `CGO_ENABLED=0` Docker build (statically linked, alpine runtime) — the driver
  must be pure Go.
- Data lives on the Railway PVC mounted at `FLUFFY_STORAGE_PATH` (500 MB quota;
  ~220 MB currently used by gob files, which stay until manually deleted).
- Game reset weekly, **Sunday 13:00 UTC** (stable per owner). Reset transitions
  are the risky moment for data continuity.
- Public SpaceTraders API only, no auth; collector is a single goroutine;
  frontend serves concurrent reads while the collector writes.
- Agent count per reset is not precisely known (leaderboards cap at ~20 visible,
  `/agents` paginates all). Current gob volume (~220 MB ≈ 2 months, compressed)
  implies roughly a few hundred agents per reset — see size math below.

## Decisions

Decided with the owner (conversation):

| Decision | Choice |
|----------|--------|
| Deployment model | Embedded, single `.db` file on the existing PVC. No hosted DB. |
| Driver | `modernc.org/sqlite` (pure Go, CGO-free). `libsql-client-go` is deleted (leftover from the abandoned Turso attempt). |
| Data migration | Fresh start at the cutover; no gob import. Old `.gob.zst` directories are left untouched — owner deletes them manually. |
| Factions | Collect: one `/factions` fetch per reset, piggybacked on the first status fetch of a new reset. |
| Retention | Whole reset weeks, not a rolling cutoff: keep 5 full game weeks; delete the just-superseded 6th week once, at the Sunday 13:00 UTC reset boundary. Never delete mid-week. |

My calls (owner can override any of these):

| Call | Rationale |
|------|-----------|
| Retention = 5 full weeks, pruned on Sunday | Matches the game structure (owner: each week is a new game). The prune and VACUUM fire once, right at the reset transition, inside the quiet window while the collector's tickers are stopped — no mid-week deletes, no contention with cycle writes. |
| `synchronous=FULL` + WAL | At this write rate (one transaction per 5 min) fsync cost is negligible, and a hard OOM kill cannot lose the last cycle. `NORMAL` risks exactly the "missed cycle" failure that matters most. |
| One transaction per collection cycle | The 5-min agent cycle (~N hundred rows) commits atomically — partial cycles cannot exist. |
| Normalize agents into a registry table | Symbol strings (~20 B) dominate a row if stored per-record. A global `agents` registry (INTEGER id ↔ symbol) halves series storage and gives per-agent index locality. See size math. |
| `WITHOUT ROWID` for series tables | The clustered PK `(agent_id, timestamp)` *is* the table — no secondary b-tree, no rowid index. Leanest layout for append-only series. |
| Same package, stable collector surface | `internal/datastore` keeps its name and its collector-facing `Store*` signatures. Frontend-facing getters change shape where the queries require (see the goals section) — grounded in the owner's "tweaking the frontend later is fine, consistent data is the requirement." |
| Busy-waits for `LatestReset`/`NextReset` stay | They work today; on first boot (fresh DB) they block until the first status cycle — same behavior as today. Builder may improve, not required. |
| Collector-facing `Store*` surface kept, frontend getters may change | The collector is the consistency-critical path; keep its call sites stable. Frontend getters may gain parameters (agent-set filter) or new forms (aggregate `LatestShips`/`LatestConstructions`) — the owner OK'd frontend changes. |
| Bounded retry on failed cycle writes | A loud error alone still drops the window — the owner called a missed cycle the worst outcome. A failed cycle's transaction is retried with backoff; timestamps are natural keys and writes are `INSERT OR REPLACE`, so retries and the next cycle coalesce idempotently. If the cycle is still unwritable when the next tick fires, the oldest buffered cycle is dropped with an escalating log — unbounded retry would block collection entirely, which is worse. Metric: `datastore_write_retries_total` alongside the error counter. |
| Free-space guard on VACUUM / export | During the transition week the old gob dirs (~220 MB) share the 500 MB PVC with a growing DB; a `VACUUM INTO` or export snapshot needs ~DB-size free space, and ENOSPC would break collection (the worst outcome). Both check free space first and skip with a log when short. Recommended (owner action): delete the gob dirs right after cutover to free ~220 MB. |
| `tools/decoder` is deleted | Its gob types disappear with the old store. The `sqlite3` CLI against the .db file replaces it (documented in INTERNAL.md). Builder may swap this for a tiny dump tool if it keeps the repo simpler. |
| `FLUFFY_WRITE_JSON` is removed | JSON queries are done with the `sqlite3` CLI against the .db file. |
| `/export` serves a consistent DB snapshot | `VACUUM INTO` to a temp file, then stream it — avoids copying a torn WAL state. |
| New env var `FLUFFY_RETENTION_WEEKS` (default 5) | Retention policy without a rebuild, in whole weeks. |
| Schema versioning via `PRAGMA user_version` | Fresh start needs only v1, but future schema changes get a home. |

## Schema (v1)

```sql
PRAGMA user_version = 1;

-- global registry: one row per agent symbol ever seen
CREATE TABLE agents (
    id         INTEGER PRIMARY KEY,
    symbol     TEXT NOT NULL UNIQUE,
    first_seen INTEGER NOT NULL
);

-- per-reset snapshot ("latest" data; upserted every agents cycle)
CREATE TABLE agent_snapshots (
    reset_date  TEXT    NOT NULL,
    agent_id    INTEGER NOT NULL REFERENCES agents(id),
    credits     INTEGER NOT NULL,
    faction     TEXT    NOT NULL,
    headquarters TEXT   NOT NULL,
    system      TEXT    NOT NULL,
    updated_at  INTEGER NOT NULL,
    PRIMARY KEY (reset_date, agent_id)
);

-- global time series, ~1 row per agent per 5-min cycle
CREATE TABLE agent_status (
    agent_id  INTEGER NOT NULL REFERENCES agents(id),
    timestamp INTEGER NOT NULL,
    credits   INTEGER NOT NULL,
    ships     INTEGER NOT NULL,
    PRIMARY KEY (agent_id, timestamp)
) WITHOUT ROWID;

-- one row per reset; upserted by the status cycle
CREATE TABLE stats (
    reset_date    TEXT PRIMARY KEY,
    market_update INTEGER NOT NULL,
    agents        INTEGER NOT NULL,
    accounts      INTEGER NOT NULL,
    ships         INTEGER NOT NULL,
    systems       INTEGER NOT NULL,
    waypoints     INTEGER NOT NULL,
    status        TEXT    NOT NULL,
    version       TEXT    NOT NULL,
    next_reset    INTEGER NOT NULL,
    last_update   INTEGER NOT NULL
);

-- leaderboards: 2 kinds x ~20 rows per reset
CREATE TABLE leaderboards (
    reset_date TEXT    NOT NULL,
    kind       TEXT    NOT NULL CHECK (kind IN ('credits','charts')),
    rank       INTEGER NOT NULL,
    symbol     TEXT    NOT NULL,
    value      INTEGER NOT NULL,
    PRIMARY KEY (reset_date, kind, rank)
) WITHOUT ROWID;

-- jumpgate list ("latest" per reset; upserted)
CREATE TABLE jumpgates (
    reset_date   TEXT    NOT NULL,
    system       TEXT    NOT NULL,
    jumpgate     TEXT    NOT NULL,
    headquarters TEXT    NOT NULL,
    status       INTEGER NOT NULL,        -- NoActivity/Active/Const/Complete
    completed_at INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (reset_date, system)
);

-- construction time series, ~1 row per active/completed gate per cycle
CREATE TABLE construction (
    jumpgate  TEXT    NOT NULL,
    timestamp INTEGER NOT NULL,
    fabmat    INTEGER NOT NULL,
    advcct    INTEGER NOT NULL,
    PRIMARY KEY (jumpgate, timestamp)
) WITHOUT ROWID;

-- factions: one fetch per reset, upserted per symbol
CREATE TABLE factions (
    reset_date    TEXT NOT NULL,
    symbol        TEXT NOT NULL,
    name          TEXT NOT NULL,
    description   TEXT NOT NULL,
    headquarters  TEXT NOT NULL,
    traits        TEXT NOT NULL,          -- JSON array
    is_recruiting INTEGER NOT NULL,
    PRIMARY KEY (reset_date, symbol)
) WITHOUT ROWID;
```

Timestamps are unix seconds (the collector already truncates/rounds to the
minute, which also makes the natural key stable on retry).

### How the hot paths map to queries

- `GetAgentHistory(reset, start, end, agents)` → `SELECT a.symbol, s.timestamp,
  s.credits, s.ships FROM agent_status s JOIN agents a ON a.id = s.agent_id
  WHERE a.symbol IN (?, …) AND s.timestamp BETWEEN ? AND ?` — the handler
  passes the chart's agent list, so only the charted series is ever in memory.
- "Latest ships per agent" (`latestShips`) → `SELECT a.symbol, s.ships,
  MAX(s.timestamp) FROM agent_status s JOIN agents a ON a.id = s.agent_id
  WHERE s.timestamp >= ? GROUP BY a.symbol` — a small aggregate instead of a
  full-history scan; likewise latest construction per gate for the overview
  tables (`GROUP BY jumpgate` with `MAX(timestamp)`).
- `GetConstructions(reset, start, end)` → `WHERE timestamp BETWEEN` on
  `construction`.
- `GetAgentList(reset)` → `agent_snapshots JOIN agents` for that reset.
- Single-value getters (`GetStats`, `GetLeaderboard`, jumpgate lookups) →
  primary-key lookups for the latest reset (`SELECT … WHERE reset_date =
  (SELECT MAX(reset_date) …)`), which makes cross-reset queries work as a
  side effect (fixes #4).

### Size math (estimate, verify empirically at implementation)

A normalized `agent_status` row is ~28 B (key + 3 integers, WITHOUT ROWID).
With a few hundred agents: ~1 M rows/week ≈ 30–40 MB/week; five reset weeks ≈
**150–200 MB for 35 days** — fits the 500 MB PVC with room even at 2× the
estimate. The gob data it replaces used ~25 MB/week *compressed*; uncompressed
SQLite is bigger per logical row but normalized, and the PVC is not the
constrained resource (RAM is).

Builder contingency, with owner consulted if hit: if measured size exceeds
~250 MB for the kept weeks, either drop retention to 4 weeks or keep 5-minute
granularity for the current reset week and hourly (decimated) rows for older
weeks. Do not silently reduce retention below 4 full weeks.

### Durability, concurrency, pragmas

```sql
PRAGMA journal_mode = WAL;      -- readers never block the writer
PRAGMA synchronous = FULL;      -- a hard kill cannot lose the last cycle
PRAGMA busy_timeout = 10000;    -- collector writes are short; overlap is rare
PRAGMA foreign_keys = ON;
PRAGMA cache_size   = -4000;    -- 4 MB page cache cap; keeps RSS flat
-- mmap stays off (default): mapped pages would count toward RSS
```

One `*sql.DB` shared process-wide (`database/sql` handles a small connection
pool; WAL permits concurrent readers with the single writer goroutine).

## Cutover and rollout

1. One PR: new datastore implementation, schema init, factions wiring (fixing
   the `updateFactionss` typo on the way), retention pruning, metrics, tests,
   deletion of the gob/zst path, `consolidate.go`, `tools/decoder`,
   `libsql-client-go`, and `FLUFFY_WRITE_JSON`. The #5 nil-check fix lands in
   the new `StoreStats`.
2. Deploy at the **Sunday 13:00 UTC reset boundary** (owner runs the deploy).
   The fresh DB starts with the new reset — any "gap" is the intentional
   retirement of the gob data.
3. Old `.gob.zst` reset directories remain on the PVC, untouched by the new
   code; owner deletes them when satisfied — **recommended in the first week**
   (frees ~220 MB; until then the free-space guard may skip VACUUM/export).
4. Watch `/debug/vars` (write-error/retry counters) and the Railway RAM graph
   for a full reset week; success = flat at the floor.

## Retention

Retention is in **whole game weeks**, not a rolling daily cutoff — each reset
week is a separate game. Keep **5 full reset weeks plus the in-progress week**
(35 days of complete games on Sunday). Nothing is ever deleted mid-week.

The prune runs **once per reset transition** (Sunday 13:00 UTC), triggered by
the collector's reset detection, inside the quiet window while the tickers are
stopped. On the transition into reset R:

- series tables (`agent_status`, `construction`):
  `DELETE … WHERE timestamp < unix(R.start − 5×7 days)` — drops exactly the
  week that just became the 6th-oldest, plus anything older (e.g. first-boot
  leftovers);
- per-reset tables (`agent_snapshots`, `stats`, `leaderboards`, `jumpgates`,
  `factions`): `DELETE … WHERE reset_date < date(R.start − 5×7 days)`.

`VACUUM` runs immediately after the prune, at the same weekly moment, subject
to the free-space guard — on a ~150 MB DB this is a few seconds of I/O, once a
week, when nothing else is writing. `FLUFFY_RETENTION_WEEKS` (default 5)
controls the count. The prune and VACUUM never touch the current reset.

## Observability

- Keep existing `datastore_*` counters; add `datastore_write_errors_total`,
  `datastore_write_retries_total`, and `datastore_prune_*` (rows deleted, last
  run).
- Failed cycle writes log loudly via `logging.Error` **and** return an error the
  collector logs per-cycle (today both sides swallow errors).
- `datastore_cache_resets_total` (dormant leftover) is removed.

## Testing

- Round-trip tests for each `Store*`/`Get*` against a temp-file DB.
- Time-window + agent-set queries: only rows inside `[start, end]` for the
  requested agents return (the #3 fix).
- Aggregate getters: `LatestShips`/latest-construction return one row per
  agent/gate with the max timestamp.
- Reset transition: write across a simulated reset boundary; both resets query
  correctly (the #4 fix).
- Retention: on a simulated reset transition, the 6th-oldest week is dropped
  and exactly 5 full weeks + the current partial week remain; nothing is
  deleted mid-week; the current reset is untouched.
- Retry path: a forced write failure is retried and coalesces idempotently
  with the next cycle (`INSERT OR REPLACE` on the natural key); the drop-oldest
  bound fires with a loud log after repeated failure.
- Free-space guard: `VACUUM INTO`/export skip cleanly (logged) when free space
  is below the threshold.
- Failure injection: unwritable DB file → errors propagate, counters increase.
- `go test -race ./...` clean (frontend still reads while collector writes).
- The existing gate tests keep passing untouched.

## What's left to the builder (implementation plan territory)

- Exact Go function signatures/struct mapping (types mostly carry over from
  `internal/datastore/types.go`; `TimedAgentRecord`/`resetAgentKey` dead types
  go away).
- How the factions fetch is triggered off reset detection in `collector.go`.
- Where the reset-transition prune hook lives (off the collector's reset
  detection) and its metric wiring.
- Whether `LatestReset`/`NextReset` keep the sleep-loop or switch to a
  condition variable (behavior must not change).
- Internal file layout of the rewritten `internal/datastore` package.

## Issue mapping

- #7 umbrella — this design; closes when #3, #4, #6 are resolved.
- #3 memory pressure — resolved by time-bounded queries (verified by flat RAM).
- #4 cross-reset reads — resolved by `reset_date` columns + PK lookups.
- #5 StoreStats nil panic — folded into the new store implementation.
- #6 factions — resolved as "collect" (factions table + wiring).