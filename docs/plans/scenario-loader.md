# Plan: Scenario Format & Loader (GUI-authored)

> **Status: DEFERRED.** Blocked on the memory system (`docs/plans/memory-system.md`), which defines what a scenario must be able to author (episodes, dossiers). SQLite is ruled out; storage is JSON.

## Context and Constraints

- **Leave `scnario/` alone.** The existing Markdown scenario stays untouched. It is not ported, renamed or used as a source.
- **The thing being replaced** is `pkg/scenario/accidental_arrangement.go`, which builds the world in Go. A second copy lives in `tools/visualizer`.
- **Scenarios are written by a GUI scenario creator, not by hand.** The format is optimized for machines: it must be easy to parse, store, validate, version and diff. Readability for human authors does not matter. Precision does.
- **Scenarios will be very large and very detailed.** They describe people's lives: identities, biographies, relationships, episodic memories and the person's own thoughts *about* those memories. They also describe locations, sub-locations, points of interest and objects, each with many attributes. Expect thousands of records and tens of MB per scenario.

---

## 1. Format Decision

### Candidates

| | JSON + JSON Schema | SQLite file | Protobuf | YAML |
|---|---|---|---|---|
| Parse in Go | stdlib | needs a driver (cgo `mattn`, or pure-Go `modernc` which is large) | codegen + `protoc` toolchain | 3rd-party lib |
| Parse in a GUI (web/Electron/Tauri) | native | wasm/native binding | `protobuf-es` codegen | lib |
| Shared contract with the GUI | JSON Schema → TS types, form generation, live validation (ajv) | SQL DDL, hand-mapped | `.proto` → TS + Go | none |
| Diff / version control / inspect | text, deterministic | binary | binary (or lossy JSON mapping) | text |
| Very large, deeply relational data | fine up to ~100 MB, whole-file load | excellent: queries, partial load, transactions | excellent | slow, ambiguous typing |
| Precision traps | duplicate keys, int > 2^53 in JS (both handled below) | none | int64/enum JSON quirks | implicit typing (`no` → false), anchors |

### Recommendation: **JSON, normalized, with a versioned JSON Schema as the contract**

- **No new dependencies on the engine side.** It's the native data format of any GUI stack.
- **One schema drives everything.** The GUI generates its TypeScript types, forms, tooltips (from field `description`s) and live validation from it. The engine decodes into matching Go types.
- **Normalized flat tables** (see §2) map one-to-one onto relational tables. If scenarios outgrow whole-file loading, moving to SQLite is a mechanical change of storage, not a redesign. The trigger to revisit is files over about 100 MB, or needing partial or queried loading of memories at runtime.
- **YAML is dropped.** Its only advantage was hand-authoring, which no longer applies.

---

## 2. Document Model

### Principles

1. **Normalized, not nested.** Each record type is a flat array of records with an `id`. Relationships between records are ID references (`parent_id`, `character_id`, `memory_ids`, …). Without nesting, the GUI edits one record at a time by ID, references are explicit and checkable, and diffs stay local.
2. **Arrays of records, never maps keyed by ID.** In JSON, object keys have no order, and duplicate keys silently win last in Go. Arrays keep authored order, and the validator rejects duplicate IDs.
3. **One canonical shape per field.** There is no shorthand (for example "a string means a public attribute"). Every field has exactly one representation.
4. **No `null`.** An absent field means the schema's documented `default`. `""` and `[]` are explicit values. This removes the absent / null / empty ambiguity.
5. **Strict.** Unknown fields are an error, so nothing the GUI wrote is silently dropped. GUI-only data (canvas positions, colors, collapsed state) goes in a reserved `editor` object, allowed on the document and on every record, which the engine ignores.
6. **Stable IDs.** The GUI generates each ID once at creation and never changes it; renaming changes `name` only. IDs are slugs (`^[a-z0-9][a-z0-9_-]{0,63}$`, unique across the whole document), **not UUIDs**. IDs end up in LLM prompts and structured-output enums, and a UUID costs about 20 tokens and is harder for the model to use reliably.
7. **JS-safe numbers.** Integers stay within ±2^53−1, so the seed is bounded. Floats are finite.
8. **In-world time is RFC 3339 UTC, plus a precision.** Memories are fuzzy, so time-bearing records carry `precision` ∈ {`year`, `month`, `day`, `hour`, `minute`} and an optional `label` such as "the summer I turned twelve". Fictional calendars are a later display concern.
9. **Subjective vs. objective, as in the engine.** Objective ground truth (world, history) is kept separate from what a character believes or remembers. Arbiter-visible and actor-visible data are never mixed in the same record.

### Top-level shape

```json
{
  "format":        { "name": "omnia.scenario", "version": "1.0" },
  "meta":          { "id": "...", "title": "...", "description": "...", "tags": [], "created_at": "...", "updated_at": "...",
                     "generator": { "name": "omnia-studio", "version": "0.4.2" } },
  "characters":    [ ],
  "entities":      [ ],
  "relationships": [ ],
  "history":       [ ],
  "memories":      [ ],
  "starts":        [ ],
  "editor":        { }
}
```

### Record types

| Record | Purpose | Key fields (abridged) | Engine use in v1 |
|---|---|---|---|
| `characters` | **Who a person is**, independent of any start point | `id`, `name`, `aliases`, `personality`, `directives[]`, `background`, `born_at`, `traits[]` (`name`, `description`, `intensity`) | → `actor.Profile` (+ traits rendered into personality) |
| `entities` | **Things in the world**. `kind` ∈ `space`, `item`, `portal`, `actor`. `class` ∈ `location`, `sub_location`, `point_of_interest`, `object`, `container` is a semantic hint for the GUI and default salience | `id`, `kind`, `class`, `name`, `aliases`, `description`, `salience`, `parent_id`, `link` (`relation`, `placement`, `concealed`), `attributes[]`, kind-specific blocks: `space` (`enterable`, `open`, `transparent`, `soundproof`), `portal` (`from_id`, `to_id`, `direction`, `transparent`, `closed`, `locked`), `actor` (`character_id`, `appearance`, `controller` ∈ `llm`/`none`) | → `sim.Entity` tree, portals, ACL attributes |
| `attributes` (embedded in entities) | Detailed state, with the ACL principle unchanged | `key`, `value` (string), `allowed[]`, `core`, `visibility`, `accessors[]` (actor entity IDs), `note` (author-only, never shown to the LLM) | → `sim.Attributes` |
| `relationships` | Directed person → person stance | `id`, `from_id`, `to_id` (characters), `label`, `summary`, `since`, `attributes[]` | Rendered into the owner's developer context (new, small) |
| `history` | **Objective** past events: what actually happened | `id`, `occurred_at`, `precision`, `location_id`, `participant_ids[]`, `summary`, `details` | Loaded; not yet fed to the Arbiter (see §6) |
| `memories` | **Subjective** episodic memory owned by one character, which may be distorted | `id`, `owner_id`, `of_history_id` (optional), `occurred_at`, `precision`, `label`, `location_id`, `participant_ids[]`, `entity_ids[]`, `content` (first-person), `emotional_tone`, `salience`, `fidelity` ∈ `accurate`/`distorted`/`false`, `reflections[]` | Needs engine support (§6) |
| `reflections` (embedded in memories) | The character's thoughts **about** that memory | `id`, `formed_at`, `kind` (thought kind), `content`, `salience` | Become thoughts linked to the memory (§6) |
| `starts` | Authored starting snapshots (§3) | see below | → initial simulation state |

The field lists above are a skeleton. The full field set is defined in the Go spec types (§4) and published through the generated schema. Phases add fields with schema descriptions; this plan does not freeze every field.

---

## 3. Start Points: Defaults and Overrides, Fully Explicit

A start point is a **snapshot of initial state** built on the shared records:

```json
{
  "id": "opening",
  "name": "Mia drags Zack upstairs",
  "clock": { "cycle": 1, "start_at": "2026-10-16T18:35:00Z", "tick_seconds": 20 },
  "seed": 42,
  "player_id": "zack",
  "present_ids": ["living-room", "kitchenette", "zack", "nelly", "mia", "..."],
  "overrides": [
    { "entity_id": "nelly", "parent_id": "living-room",
      "link": { "relation": "in" },
      "set_attributes": [ { "key": "mood", "value": "Deeply flustered", "visibility": "public" } ],
      "remove_attributes": [] }
  ],
  "thoughts": [
    { "id": "t-nelly-1", "owner_id": "nelly", "kind": "intention", "content": "...", "salience": 0.95,
      "entity_ids": ["relationship-notes"], "memory_ids": [] }
  ],
  "events": [ { "id": "e-open", "tick": 1, "location_id": "living-room", "text": "..." } ]
}
```

These rules are exact, so a GUI and the engine compute the same state:

- **Entities.** Only entities listed in `present_ids` exist in this start, plus the ancestors and portals they need; the validator enforces closure. An absent `present_ids` means all entities.
- **Overrides.** A field-level replace of `parent_id`, `link` or `salience`. Attributes are replaced per key with whole records; there is no merging inside one attribute. `remove_attributes` deletes keys and is rejected for `core` keys.
- **Memories** included are those owned by present characters with `occurred_at` ≤ `clock.start_at`, which gives time travel across start points for free. `memory_exclude_ids[]` allows exceptions.
- **The player** is optional. A start with no player is a fully autonomous run. A runtime `-player` override is allowed.

This is the same shape a future **save game** needs (an engine-produced snapshot), so v1 deliberately avoids anything start-specific that a save couldn't express.

---

## 4. Schema as the Single Source of Truth

- **Go spec types are authoritative:** `pkg/scenario/spec` holds plain structs, depends on neither `sim` nor `llm`, and is tagged with `json` plus `jsonschema` tags (description, enum, pattern, min/max, default).
- **The schema is generated:** `tools/schemagen` emits `schema/omnia.scenario.v1.json` (JSON Schema 2020-12). It is committed, and a golden test fails if it is stale.
- **The GUI consumes the schema:** TypeScript types (`json-schema-to-typescript`), forms and tooltips from descriptions, and live shape validation (ajv). Semantic checks come from the engine's validator through a CLI (§5).
- **Engine enums live in one place.** Relations, thought kinds, visibility and effect-relevant enums come from the same Go constants the engine uses, so the GUI can never offer a value the engine rejects.
- **Reference docs are generated:** `docs/scenario-format.md` is produced from the schema descriptions, so it can't drift.

---

## 5. Loader Pipeline

```
bytes ─▶ Decode ─▶ spec.Document ─▶ Migrate ─▶ Validate ─▶ Build(opts) ─▶ *sim.Simulation + Instance
        (pure)                      (pure)      (pure)       (only stage touching sim/actor/llm)
```

### Decode (`pkg/scenario/decode.go`)

1. **Size limit.** The default is 256 MB and is configurable. Gzip is detected transparently by magic bytes (`.omnia.json` or `.omnia.json.gz`).
2. **Version gate before strict decoding.** Read `format` with a lenient pass first. An unknown major version is rejected. A newer minor version is rejected with "requires format 1.3; engine supports ≤ 1.2", because strict mode would otherwise report confusing unknown-field errors.
3. **Duplicate-key scan.** A streaming token pass over `json.Decoder` rejects duplicate object keys, since stdlib silently takes the last one. It is cheap and linear.
4. **Strict typed decode** with `DisallowUnknownFields`. `editor` is typed `json.RawMessage` and passed through untouched.

### Migrate (`pkg/scenario/migrate`)
These are ordered `vN → vN+1` functions that operate on generic JSON before the typed decode. The GUI can call `omnia scenario migrate`, so both sides upgrade old files identically.

### Validate (`pkg/scenario/validate.go`)
It runs every check and returns all issues together, with no network calls. Each issue carries a **JSON Pointer** (`/entities/412/attributes/3/accessors/0`), the record `id`, a severity, a code and a message. The pointer lets the GUI highlight the exact field.

| Category | Checks |
|---|---|
| Identity | Unique IDs across the document; slug pattern; `meta.id` set |
| References | Every `*_id`/`*_ids` resolves **and points at the right kind**: `accessors` → actor entities, `character_id` → characters, `owner_id` → characters with an actor entity, `location_id` → spaces, portal `from_id`/`to_id` → spaces |
| Tree | Acyclic; roots are spaces; actors inside enterable spaces (or `seated`/`on` an item inside one); valid `relation` per parent kind; non-enterable spaces only inside items or actors |
| Attributes | Unique `key` per entity; `value` ∈ `allowed` when set; core `arousal` ∈ {low, normal, high}; ACL shape |
| Time | Memory and history `occurred_at` ≤ the latest start's `start_at` (warning); a character's `born_at` ≤ their memories |
| Starts | `present_ids` closure (ancestors, portal endpoints); override targets present; `player_id` is a present actor; thoughts reference present entities and included memories; events `tick ≥ 1` at present spaces; `tick_seconds > 0` |
| Numbers | Salience ∈ [0.05, 1]; integers JS-safe |

The CLI `omnia scenario validate <file> [--json]` is what the GUI calls. Its exit code is non-zero on errors, and `--json` outputs the issue list.

### Build (`pkg/scenario/build.go`)

```go
type BuildOptions struct {
    Client      llm.Client   // required; injected, the loader never reads env or credentials
    Model       string
    Start       string       // "" = first start
    Player      string       // overrides start.player_id; "" = as authored
    Interactive bool         // attach a HumanController to the player
    Seed        *int64
    SimOptions  []sim.Option // e.g. traced components for the visualizer
}

type Instance struct {
    World     *sim.Simulation
    HumanCtrl *sim.HumanController     // nil unless Interactive
    Player    sim.EntityID             // "" if autonomous
    Actors    map[sim.EntityID]*actor.Actor
    Doc       *spec.Document
}
```

The builder resolves the start state (§3) into an in-memory resolved world, then calls the existing public API (`SpawnSpace`/`SpawnEntity`/`SpawnActor`/`SpawnPortal`/`Move`, `Attributes.Set*`, `AddThought`, `ScheduleEvent`). It **checks every error**, which the current hardcoded setup mostly ignores. A required engine fix: `sim.New` must stop silently calling `llm.DefaultClient()` when components are injected.

---

## 6. Engine Work This Format Depends On

The format can describe more than the engine uses today. Each gap gets an explicit decision rather than silently loaded but unused data.

| Gap | v1 decision |
|---|---|
| **Episodic memories and reflections**: `actor.Memory` has only thoughts and turn history | **In scope, minimal.** Add `Episode` records to `actor.Memory`. Each tick, retrieve the top-K episodes by overlap with perceived and focused entity IDs, salience and recency, into a developer message `YOUR RELEVANT MEMORIES`. Reflections become thoughts linked to their memory ID. The Distiller's link enum also accepts memory IDs. This could be split into its own plan if it grows |
| **Relationships** | **In scope, small.** Render the owner's outgoing relationships for characters currently perceived |
| **Objective `history`** | **Loaded, not used.** Arbiter use (a digest of relevant past events) is a follow-up; the validator marks it with an info-level note |
| **Nested enterable spaces** (sub-locations) | **To verify in Phase 0.** Check how scenes, LOD perception and portals treat an enterable space inside an enterable space. If unsupported, the validator rejects `sub_location` as enterable until engine support lands; as a POI-style non-enterable space it is fine |
| **Typed attribute values** (numbers, units, ranges) | **Out of v1.** Values stay strings to match `sim.Attributes`; typed values are a later minor version |

---

## 7. Phases

Each phase is one commit, and `go vet ./... && go test -race ./...` stays green after each. `scnario/` is untouched throughout.

0. **Decisions and engine checks.** Settle the open questions (§9) and verify nested-space semantics.
1. **Spec types and schema generation.** `pkg/scenario/spec`, `tools/schemagen`, the committed schema, and a golden test for schema freshness.
2. **Decoder.** Size limit, gzip, version gate, duplicate-key scan, strict decode, `editor` passthrough. Tests use fixtures for each failure mode.
3. **Validator and CLI.** All rules with JSON-Pointer issues, plus `omnia scenario validate [--json]`. Table tests, one fixture per rule.
4. **Engine: memories and relationships** (§6). Covers the store, retrieval, prompt rendering, memory IDs as thought links, and tests with a fake LLM.
5. **Builder.** Start-state resolution, `BuildOptions`/`Instance`, and the `sim.New` default-client fix. Tests check the tree, ACLs, thoughts, memory inclusion by time, events, controller wiring and the player override.
6. **Fixtures, parity and stress.**
   - `tools/fixturegen` writes scenario documents with Go code, acting as a stand-in GUI. It produces `scenarios/accidental_arrangement.omnia.json` from the **current hardcoded Go setup**, not from `scnario/`.
   - A **parity test** builds both and compares entities, parents, links, attributes with ACLs, thoughts and events. Then `pkg/scenario/accidental_arrangement.go` is deleted.
   - A **stress generator** creates about 50 characters × 500 memories, about 5k entities and about 30 attributes each. A benchmark target is decode, validate and build in under 2 s and under 1 GB RSS. If that target is missed, the SQLite decision is revisited (§1).
7. **Entry points.**
   - `main.go` and `cmd/omnia-http` get `-scenario <file>`, `-start`, `-player`; the hardcoded `"zack"`, `NellyMind` and title strings are removed.
   - The HTTP server and banners read the title and player from `Instance`.
   - `.env` loading moves to `internal/env`, and `tools/visualizer` is moved onto the loader.
8. **Docs.** Generated `docs/scenario-format.md`, plus a **GUI integration guide**: schema consumption, canonical write rules (UTF-8, LF, schema key order, no nulls, JS-safe integers), the `editor` namespace, calling `validate`/`migrate`, and error pointers.

## 8. Acceptance Criteria

1. `pkg/scenario` contains no scenario-specific content; a grep test checks this.
2. The JSON Schema is generated from the Go spec types and its freshness is test-enforced. A document that passes the schema but fails semantics is caught by `validate` with exact JSON Pointers.
3. The Accidental Arrangement start loaded from JSON matches today's hardcoded world, per the parity test.
4. A new scenario runs with **zero Go changes**.
5. The stress fixture meets the performance budget.
6. The loader never touches env vars or credentials, and `scnario/` is unmodified.

## 9. Open Questions

- **Q1. Format:** JSON + JSON Schema (recommended) vs. SQLite as the primary storage.
- **Q2. Schema generation:** an in-house reflection generator (no dependency, about 300 lines) vs. `github.com/invopop/jsonschema` (a dependency, less code). Either way the Go types stay authoritative.
- **Q3. Memory engine work (Phase 4):** keep it inside this plan, or split it into its own plan and ship the loader with memories loaded but not yet used?
- **Q4. File packaging:** a single document file (recommended), or a bundle (zip or directory) to reserve room for future binary assets such as portraits and maps?
- **Q5. GUI stack:** if already decided (web, Tauri, native), it confirms or changes the JSON Schema toolchain assumptions in §4.
