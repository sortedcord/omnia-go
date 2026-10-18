# Plan: Scenario Format & Loader

> **Status: ACTIVE.** Unblocked: the three-tier memory system (`docs/plans/memory-system.md`) is implemented, and it defines what a scenario can author about a mind. SQLite is ruled out; storage is JSON.

## Context and Constraints

- **Leave `scnario/` alone.** The Markdown scenario stays untouched. It is not ported, renamed or used as a source.
- **What gets replaced.** The hand-written Go scenario in `pkg/scenario/`:
  - `accidental_arrangement.go`: clock, actors, profiles, controllers, opening event.
  - `world_topology.go`: 23 spaces, portals, props and ACL attributes.
  - `episodes_nelly.go`, `episodes_zack.go`, `episodes_mia.go`: 15 authored episodes.
  - `dossiers.go`: subjects (including phantoms), bindings and claims.

  A second, older copy of the setup also lives in `tools/visualizer`.
- **Scenarios are templates.** A scenario file is read-only input. The engine never writes it back; a running simulation diverges into memory and, later, into separate save files (§3).
- **No GUI for now.** A GUI scenario creator is deferred. Scenarios are written as JSON (by hand or by Go fixture generators) and checked with the CLI validator. The format stays machine-friendly (easy to parse, validate, version and diff), so a GUI can be added later without a format change.
- **Scenarios will be large.** They describe people's lives (identities, biographies, episodic memories, beliefs about other people and things) and detailed worlds (buildings, rooms, objects, ACL attributes). Expect thousands of records and tens of MB per scenario.

### What hand-authoring the current scenario taught us

Each of these is a requirement on the format, not a nice-to-have:

| Lesson | Consequence for the format |
|---|---|
| The opening was rewritten three times. Moving Zack and Mia outside a closed door split the world into two scenes. | Portal state (`closed`, `locked`, `transparent`) and actor placement are first-class start data. They decide the initial scene partition. |
| Scripted dialogue in opening events railroaded the player and NPCs. | Opening events describe **situation, not speech or decisions**. The validator emits a lint warning for quoted speech in `events[].text`. |
| Actors with no inbox events take no turn (Nelly waits silently until something reaches her). | `events` are the only thing that decides **who acts first**. `validate` reports which actors each tick-1 event reaches. |
| Recall only works if dossiers link to episodes. Without links, mention cues surfaced nothing. | Links derive mechanically from episode `participant_ids` / `place_ids` and claim `evidence`. They are never authored by hand. |
| Subject kind was guessed from names ("Pilot" became a place). | Subject `kind` is explicit, and the world entity `kind` is authoritative for auto-registration. |
| Strangers must not share memories ("Zack and Nelly have never met"). | It is an authoring lint in `validate`: a warning when a character appears as a participant in another character's episode. It is never an engine rule. |
| Intentions and feelings depend on the start point ("hear Mia out outside the door"). | Minds are split into **lifetime memory** (per character) and **start-specific state of mind** (per start). |

---

## 1. Format Decision

**A single JSON file, normalized, with a versioned JSON Schema as the contract.** This is decided. SQLite, YAML and multi-file bundles are ruled out.

- **One file per scenario** (`*.omnia.json`). No bundles, no compression, no external assets.
- **One schema drives everything.** Editors with JSON Schema support (e.g. VS Code via `$schema`) give autocomplete, tooltips (from field `description`s) and shape validation for free. The engine decodes into matching Go types. A future GUI consumes the same schema.
- **Normalized flat tables (§2)** map one-to-one onto relational tables. If scenarios outgrow whole-file loading, moving to SQLite is a mechanical storage change, not a redesign. The trigger to revisit is files over ~100 MB, or needing partial or queried loading of memories at runtime.
- **The memory snapshot is already JSON** (`memory.Snapshot` in `pkg/memory/codec.go`). Scenario minds are a strict subset of it, so a future save game is the same shape plus runtime fields.

---

## 2. Document Model

### Principles

1. **Normalized, not nested.** Each record type is a flat array of records with an `id`. Relationships are ID references. References are explicit and checkable, and diffs stay local.
2. **Arrays of records, never maps keyed by ID.** Arrays keep authored order. The validator rejects duplicate IDs (Go silently keeps the last duplicate key in a map).
3. **One canonical shape per field.** No shorthand forms.
4. **No `null`.** An absent field means the schema's documented `default`. `""` and `[]` are explicit values.
5. **Strict.** Unknown fields are an error. The only exceptions are the optional top-level `$schema` string (editor support; ignored by the engine) and a reserved `editor` object for author or tooling notes, allowed on the document and on every record. The engine ignores it (reserved for a future GUI's canvas positions and similar).
6. **Stable slug IDs** (`^[a-z0-9][a-z0-9_-]{0,63}$`), chosen once and never changed; renaming changes `name` only. They are not UUIDs: IDs appear in LLM prompts and structured-output enums, where a UUID costs ~20 tokens and is harder for the model to use reliably.
7. **Two ID scopes.**
   - **World scope:** characters, entities, starts and events are unique across the document.
   - **Mind scope:** subjects, claims and episodes are unique **within one owner's mind**. A subject ID may deliberately equal a world entity ID (`"zack"`, `"living-room"`); that is a naming convention, not a binding. Bindings are separate and explicit.
8. **JS-safe numbers.** Integers within ±2^53−1; floats finite.
9. **In-world time is RFC 3339 UTC plus a precision** ∈ {`minute`, `hour`, `day`, `month`, `year`} and an optional `label` ("Valentine's Day moving into Apt 3B"). This matches `memory.Precision`.
10. **Subjective vs objective, as in the engine.** World records (`entities`, `history`) and mind records (`minds`) never reference each other except through **bindings**. Bindings are fallible beliefs, checked only for syntax (memory invariants I1–I3).

### Top-level shape

```json
{
  "$schema":    "./schema/omnia.scenario.v1.json",
  "format":     { "name": "omnia.scenario", "version": "1.0" },
  "meta":       { "id": "...", "title": "...", "description": "...", "tags": [],
                  "created_at": "...", "updated_at": "...",
                  "generator": { "name": "omnia-studio", "version": "0.4.2" } },
  "characters": [ ],
  "entities":   [ ],
  "history":    [ ],
  "minds":      [ ],
  "starts":     [ ],
  "editor":     { }
}
```

### World records

| Record | Purpose | Key fields (abridged) | Engine target |
|---|---|---|---|
| `characters` | **Who a person is**, independent of any start | `id`, `name`, `aliases[]`, `personality`, `background`, `born_at`, `traits[]` (`name`, `description`, `intensity`), `likes[]` / `dislikes[]` (each with a `reason`, so they stay purposeful, not generic) | `actor.Profile` (traits, likes and dislikes are rendered into `Personality` / `Background`) |
| `entities` | **Things in the world.** `kind` ∈ `space`, `item`, `portal`, `actor`. `class` ∈ `building`, `location`, `room`, `point_of_interest`, `object`, `container` is an authoring hint and sets default salience | `id`, `kind`, `class`, `name`, `aliases[]`, `description`, `salience`, `parent_id`, `link` (`relation` ∈ `in`/`on`/`under`/`behind`/`worn`/`held`/`seated`/`contained`, `placement`, `concealed`), `attributes[]`, and kind blocks: `space` (`enterable`, `open`, `transparent`, `soundproof`); `portal` (`from_id`, `to_id`, `direction`, `transparent`, `closed`, `locked`); `actor` (`character_id`, `appearance`) | `sim.Entity` tree, `Space`, `Portal`, `Link`, `Attributes` |
| `attributes` (embedded) | Detailed state with ACL, unchanged from the engine | `key`, `value` (string), `allowed[]`, `core`, `visibility` ∈ `public`/`private`, `accessors[]` (actor entity IDs) | `sim.Attribute` |
| `history` | **Objective** past events: what actually happened | `id`, `occurred_at`, `precision`, `location_id`, `participant_ids[]`, `summary` | Loaded, **not used** in v1 (§6) |

Portals are one-directional, as in the engine (`counter-pass` / `counter-pass-return`). The validator warns when a pair disagrees on `closed` or `locked`.

### Mind records (one per character)

`minds[]` holds each character's **lifetime** subjective memory: everything true of them at any start. It mirrors `pkg/memory` exactly, minus runtime-only fields.

```json
{
  "owner_id": "nelly",
  "subjects": [
    { "id": "david", "kind": "person", "names": ["David", "ex-boyfriend"], "origin": "authored", "bindings": [] },
    { "id": "zack",  "kind": "person", "names": ["Zack", "Mia's friend"],  "origin": "perceived",
      "bindings": [ { "entity_id": "zack", "mode": "identity", "confidence": 1.0 } ] }
  ],
  "dossiers": [
    { "subject_id": "david", "impression": "The man who promised we'd beat the system, then left.",
      "claims": [
        { "id": "c-nel-dav-1", "kind": "belief", "content": "...", "salience": 0.88, "conviction": 0.98,
          "fixed": true, "evidence": ["ep-nelly-david-abandonment"] }
      ] }
  ],
  "episodes": [
    { "id": "ep-nelly-david-movein", "start": "2026-02-14T19:30:00Z", "end": "2026-02-14T22:30:00Z",
      "precision": "hour", "label": "Valentine's Day moving into Apt 3B",
      "place_ids": ["living-room"], "participant_ids": ["david"],
      "title": "...", "summary": "...", "tone": "warm and hopeful", "importance": 0.90,
      "beats": [ { "text": "...", "subject_ids": ["david", "coffee-table"] } ] }
  ]
}
```

| Field | Maps to | Notes |
|---|---|---|
| `subjects[]` | `memory.Subject` | `kind` ∈ `person`/`place`/`object`/`group`/`idea`. `origin` ∈ `authored`/`perceived` (`told` is reserved). Subjects with no bindings are **phantoms** (David, the housing authority, Professor Harrison) |
| `subjects[].bindings[]` | `memory.Binding` | `mode` ∈ `identity`/`anchor`, `confidence` ∈ [0, 1]. `entity_id` may point at an entity that is absent from this start or from the world entirely. That is the de-sync feature, not an error |
| `dossiers[]` | `memory.Dossier` | One per subject at most. The **self-dossier** has `subject_id == owner_id` (non-relational feelings, intentions, self-image) |
| `dossiers[].claims[]` | `memory.Claim` | `kind` ∈ `belief`/`feeling`/`suspicion`/`intention`. `salience` ∈ [0.05, 1], `conviction` ∈ [0, 1], `fixed` (protected from the Distiller and from decay below its floor), `evidence[]` (episode IDs in the same mind) |
| `episodes[]` | `memory.Episode` | Always `source: authored`, `status: closed`. `importance` ∈ [0.05, 1]. Beats carry `text` and `subject_ids`. `from_seq` / `to_seq` are omitted (authored beats have no turns) |

**Not authorable** (runtime-only, rejected by the strict schema): dossier `links`, claim `history` / `formed_tick` / `updated_tick`, episode `recall_count` / `last_recalled_*` / `scene_id` / `prev_id`, `perceived_ids`, working-buffer turns and the ID allocator counters.

- **Links are derived at build time**, exactly as `Mind.AddEpisode` and `Mind.AddClaim` do today:
  - participants → `participant`, weight 1.0
  - places → `place`, weight 0.8
  - claim evidence → `evidence`, weight 1.0
- **`prev_id`** is derived by sorting each mind's episodes by `start`.

---

## 3. Start Points

A start is a **snapshot of initial state** built on the shared records. It also carries the **state of mind for that moment**.

```json
{
  "id": "outside-3b",
  "name": "Zack arrives at Bryn Hall",
  "clock": { "cycle": 1, "start_at": "2026-10-16T18:35:00Z", "tick_seconds": 20 },
  "seed": 42,
  "player_id": "zack",
  "present_ids": ["breezeway-3f", "living-room", "zack", "mia", "nelly", "..."],
  "overrides": [
    { "entity_id": "zack", "parent_id": "breezeway-3f", "link": { "relation": "in" },
      "set_attributes": [ { "key": "mood", "value": "Mildly bewildered, dryly amused", "visibility": "public" } ],
      "remove_attributes": [] },
    { "entity_id": "breezeway-to-apt", "portal": { "closed": true, "locked": false } }
  ],
  "minds": [
    { "owner_id": "zack",
      "episode_exclude_ids": [],
      "upsert_claims": [
        { "subject_id": "zack", "claim": { "id": "c-zk-self-1", "kind": "intention",
          "content": "Find out why Mia dragged me all the way across campus to married student housing.",
          "salience": 0.95, "conviction": 0.85 } }
      ],
      "retract_claim_ids": [],
      "upsert_subjects": [] }
  ],
  "events": [
    { "id": "e-arrive", "tick": 1, "location_id": "breezeway-3f",
      "text": "You climb the concrete stairs to the third floor of Bryn Hall... she turns her head, spots you, and a knowing grin spreads across her face." }
  ]
}
```

These rules are exact and deterministic:

- **Entities.** Only entities in `present_ids` exist, plus the ancestors and portal endpoints they need. The validator enforces this closure. An absent `present_ids` means all entities.
- **Overrides.** A field-level replace of `parent_id`, `link`, `salience`, or the `space` / `portal` blocks.
  - Attributes are replaced per key with whole records; there is no merging inside one attribute.
  - `remove_attributes` deletes keys and is rejected for `core` keys.
- **Episodes included** are a character's lifetime episodes with `end` ≤ `clock.start_at`, minus `episode_exclude_ids`. This gives time travel across starts for free: an earlier start simply hasn't "lived" later episodes yet.
- **Claims.** The lifetime dossiers are applied first, then the start's `upsert_claims` (replace by claim ID, or insert), then `retract_claim_ids`.
  - Claims whose `evidence` points only at excluded episodes are **kept, with that evidence dropped**. A belief can outlive the memory that formed it; the validator emits an info note.
- **Subjects** in `upsert_subjects` replace a lifetime subject by ID (e.g. a binding that is stale only at this start).
- **Events** are the only wake-up mechanism: each event reaches the actors in its location and in spaces connected through open portals. `validate --start` lists **who acts on tick 1**.
- **The player** is optional. A start with no player is a fully autonomous run. A runtime `-player` override is allowed.

**Templates, not state.** The scenario file is never modified by the engine. Each start is a small overlay on the shared records (decided, Q6), so a character's lifetime memory is written once. Runtime state (turns, counters, links, recall bookkeeping, world mutations) lives only in memory and, later, in a separate save file that references the scenario by `meta.id` and `format.version`. Save games are out of scope for this plan.

---

## 4. Schema as the Single Source of Truth

- **Go spec types are authoritative.** `pkg/scenario/spec` holds plain structs and depends on neither `sim`, `memory` nor `llm`. They carry `json` tags plus `jsonschema` tags (description, enum, pattern, min/max, default).
- **Engine enums are defined once.** Relations, subject and claim kinds, binding modes, visibility and precision are mirrored from the engine constants. A test asserts the spec enums equal the engine enums, so the schema can never accept a value the engine rejects.
- **The schema is generated with `github.com/invopop/jsonschema`** (decided, Q2) by reflecting the spec types. `tools/schemagen` emits `schema/omnia.scenario.v1.json` (JSON Schema 2020-12). It is committed, and a golden test fails if it is stale.
- **Consumers:** scenario files declare `"$schema"` for editor support; semantic checks come from the engine validator through the CLI (§5). Enum constraints, patterns, ranges and the no-unknown-fields rule are expressed with `jsonschema` struct tags and `additionalProperties: false`.
- **Reference docs are generated.** `docs/scenario-format.md` is produced from the schema descriptions, so it can't drift.

---

## 5. Loader Pipeline

```
bytes ─▶ Decode ─▶ spec.Document ─▶ Migrate ─▶ Validate ─▶ Build(opts) ─▶ Instance
        (pure)                       (pure)     (pure)      (only stage touching sim/actor/memory/llm)
```

### Decode (`pkg/scenario/decode.go`)

1. **Size limit**, default 256 MB, configurable. Plain JSON only (`.omnia.json`).
2. **Version gate before the strict decode.** A lenient pass reads `format` first.
   - An unknown major version is rejected.
   - A newer minor version is rejected with "requires format 1.3; engine supports ≤ 1.2". Otherwise the strict decode would report confusing unknown-field errors.
3. **Duplicate-key scan.** A streaming token pass rejects duplicate object keys.
4. **Strict typed decode** with `DisallowUnknownFields`. `editor` is `json.RawMessage` and passed through untouched.

### Migrate (`pkg/scenario/migrate`)

Ordered `vN → vN+1` functions on generic JSON, run before the typed decode. `omnia scenario migrate <file>` writes an upgraded copy; it never edits in place.

### Validate (`pkg/scenario/validate.go`)

Every check runs, and all issues are returned together, with no network calls. Each issue carries:
- a **JSON Pointer** (`/minds/1/dossiers/3/claims/0/evidence/0`), so tools can point at the exact field
- the record `id`
- a severity (`error` / `warning` / `info`)
- a stable code and a message

| Category | Checks |
|---|---|
| Identity | World-scope IDs unique across the document; mind-scope IDs unique per owner; slug pattern; `meta.id` set |
| World references | Every world `*_id` resolves to the right kind: `accessors` → actor entities, `character_id` → characters, `location_id` → spaces, portal `from_id` / `to_id` → spaces, `minds[].owner_id` → a character with an actor entity |
| Tree | Acyclic; roots are spaces; actors inside enterable spaces (or `seated` / `on` an item inside one); non-enterable spaces only inside items or actors; valid `relation` per parent kind |
| Attributes | Unique `key` per entity; `value` ∈ `allowed` when set; core `arousal` ∈ {low, normal, high}; ACL shape |
| Mind structure (I1) | `dossiers[].subject_id`, `episodes[].participant_ids` / `place_ids`, and beat `subject_ids` resolve to subjects **in the same mind**, or the owner (self). Claim `evidence` resolves to episodes in the same mind. At most one dossier per subject. Beats are non-empty |
| Mind ↔ world (I2) | Binding **syntax only** (`mode`, `confidence` range). A binding to an entity absent from a start or from the world is an `info` note ("bound to an entity not present in this start"), **never** an error |
| Time | Episode `start` ≤ `end`; `born_at` ≤ the earliest episode; history `occurred_at` ≤ the latest start (warning) |
| Starts | `present_ids` closure; override targets present; `player_id` is a present actor; mind overrides reference existing owners, subjects and claim IDs; a `fixed` lifetime claim may be replaced only by explicit upsert (warning); events have `tick ≥ 1` at present spaces; `tick_seconds > 0` |
| Authoring lints | Quoted speech in `events[].text` (warning: scripted dialogue); an event that wakes no actor (warning); a start where no actor wakes on tick 1 (error); a portal pair with mismatched state (warning); a character appearing in another character's episode while the start's claims say they are strangers (warning) |
| Numbers | Salience ∈ [0.05, 1]; importance ∈ [0.05, 1]; conviction and confidence ∈ [0, 1]; integers JS-safe |

The CLI `omnia scenario validate <file> [--start id] [--json]` is the authoring loop. Its exit code is non-zero on errors, and `--json` outputs the issue list.

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
    HumanCtrl *sim.HumanController // nil unless Interactive
    Player    sim.EntityID         // "" if autonomous
    Actors    map[sim.EntityID]*actor.Actor
    Start     *spec.Start
    Doc       *spec.Document
}
```

The builder resolves the start (§3) into an in-memory resolved world and resolved minds. It then calls only the existing public APIs:
- **World:** `SpawnSpace`, `SpawnEntity`, `SpawnActor`, `SpawnPortal`, `Move`, `Attributes.Set*`, `ScheduleEvent`.
- **Minds:** `Mind.AddSubject`, `Mind.AddEpisode` (in `start` order), `Mind.AddClaim`. These derive the links.

It **checks every error**, which the current hand-written setup mostly ignores (`_, _ = world.SpawnPortal(...)`).

Required engine fixes, all small:
1. **No default client.** `sim.New` must stop silently calling `llm.DefaultClient()` when components are injected.
2. **Allocator counters.** After seeding, a mind's subject, claim and episode counters must sit past any authored ID that matches the allocator patterns (`s-<owner>-N`, `c-<owner>-N`, `ep-<owner>-N`). The allocators are already collision-free; a test pins this for authored IDs.
3. **Portal state.** `SpawnPortal` takes the authored `closed` / `locked` / `transparent` instead of the builder mutating `Portal` after spawning.
4. **Entity description and salience.** `SpawnPortal` and `SpawnActor` accept them directly instead of the builder setting fields afterwards.

---

## 6. Engine Gaps: Explicit Decisions

| Gap | v1 decision |
|---|---|
| ~~Episodic memories and reflections~~ | **Resolved** by the memory system. Authored `minds` replace the old `memories` / `reflections` / `thoughts` records |
| **Relationships** (person → person stance) | **Dropped as a separate record.** A relationship is a dossier about a person subject: an impression plus claims. One concept, not two |
| **Objective `history`** | **Loaded, not used.** It feeds no prompt; memory invariant I3 keeps the Arbiter away from minds, but a future Arbiter digest of past *world* events is allowed. Episodes may reference a history record only through `editor` (author bookkeeping, e.g. "this is Nelly's distorted memory of h-04"). The engine never reads it |
| **Nested enterable spaces** (building → floor → room) | **To verify in Phase 0.** Check how scenes, LOD perception and portals treat an enterable space inside an enterable space. Today's 23-space world is flat spaces joined by portals, which works. If nesting is unsupported, the validator rejects enterable spaces inside enterable spaces, and `building` stays an authoring grouping (`editor`) |
| **Typed attribute values** | **Out of v1.** Values stay strings, matching `sim.Attributes` |
| **Hearsay** (`source: hearsay`, `origin: told`) | **Reserved.** The schema rejects them until the memory system implements them |

---

## 7. Phases

Each phase is one commit. `go vet ./... && go test -race ./...` stays green after each, and `scnario/` is untouched throughout.

0. **Decisions and engine checks.** Settle the open questions (§9), verify nested-space semantics, and land the small engine fixes from §5 Build (default client, allocator counters, portal and actor spawn params).
1. **Spec types and schema generation.** `pkg/scenario/spec`, the enum-parity test against `sim` / `memory`, `tools/schemagen` on `invopop/jsonschema`, the committed schema and its golden freshness test.
2. **Decoder.** Size limit, version gate, duplicate-key scan, strict decode, `editor` passthrough. Fixtures for each failure mode.
3. **Validator and CLI.** All rules with JSON Pointer issues, including the mind-scope rules and the authoring lints; `omnia scenario validate [--start] [--json]`. One table test and one fixture per rule.
4. **Builder.**
   - Start resolution (entities, overrides, episode inclusion by time, the claim overlay), plus `BuildOptions` / `Instance`.
   - Tests cover: the tree, ACLs, portal state, scene partition at tick 1, mind seeding (subjects, phantoms, bindings, dossiers, episodes, derived links), events, controller wiring and the player override.
5. **Fixtures and parity.**
   - `tools/fixturegen` (Go code that writes scenario documents) produces `scenarios/accidental_arrangement.omnia.json` from the **current hand-written Go scenario**.
   - A **parity test** builds both and compares, using `memory.ExportSnapshot` for minds:
     - entities, parents, links, attributes with ACLs
     - portal state
     - subjects, bindings, dossiers, claims and episodes
     - derived links
     - events
   - The recall probes from `pkg/scenario/scenario_test.go` (a mention of "David" surfaces the abandonment episode; "Harrison" surfaces the lecture episode) run against the loaded scenario.
   - Then the hand-written scenario files are deleted.
6. **Stress.** A generator creates ~50 characters × 500 episodes, ~5k entities and ~30 attributes each. The benchmark target is decode, validate and build in under 2 s and under 1 GB RSS. If it is missed, the SQLite decision is revisited (§1).
7. **Entry points.**
   - `main.go` and `cmd/omnia-http` get `-scenario <file>`, `-start` and `-player`. The hardcoded `"zack"`, `NellyMind` and title strings are removed.
   - Banners and the HTTP server read the title, setting and player from `Instance`. The setting is the player's starting space, not a hardcoded room.
   - `.env` loading moves to `internal/env`, and `tools/visualizer` moves onto the loader, deleting its duplicate setup.
8. **Docs.** Generated `docs/scenario-format.md`, plus an **authoring guide** covering:
   - using `$schema` for editor support
   - canonical write rules (UTF-8, LF, schema key order, no nulls, JS-safe integers)
   - the `editor` namespace and the two ID scopes
   - lifetime minds vs start overlays
   - authoring guidance (opening events describe situations, not dialogue; events decide who acts first)
   - calling `validate` / `migrate`, and error pointers

---

## 8. Acceptance Criteria

1. `pkg/scenario` contains no scenario-specific content; a grep test checks this.
2. The JSON Schema is generated from the Go spec types, its freshness is test-enforced, and its enums equal the engine enums. A document that passes the schema but fails semantics is caught by `validate` with exact JSON Pointers.
3. The Accidental Arrangement start loaded from JSON matches the hand-written Go scenario, per the parity test, including mind snapshots and derived links. The recall probes pass.
4. A new scenario, including new characters with authored episodes and dossiers, runs with **zero Go changes**.
5. The validator **never** errors on a binding to an absent entity (memory invariant I2).
6. The stress fixture meets the performance budget.
7. The loader never touches env vars or credentials, and `scnario/` is unmodified.

---

## 9. Decisions

| # | Decision |
|---|---|
| Q1 | Format: **JSON + JSON Schema** |
| Q2 | Schema generation: **`github.com/invopop/jsonschema`**, reflecting the Go spec types |
| Q3 | Memory engine work: **done** in `pkg/memory`; this plan only seeds it |
| Q4 | Packaging: **a single `.omnia.json` file**; no bundles, no compression, no assets |
| Q5 | GUI: **deferred**. Authoring is JSON plus `$schema` editor support plus `omnia scenario validate`. The format stays GUI-ready (schema, `editor` namespace, JSON Pointer issues) |
| Q6 | A scenario file is a **read-only template**. Starts are overlays on lifetime records; the engine never writes the file; runtime state goes to separate save files (out of scope) |
| Q7 | Subject IDs **may equal entity IDs** (current convention). Bindings stay explicit; equal IDs imply nothing |
