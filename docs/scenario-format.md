# Omnia Scenario Specification & Authoring Guide

> **Specification Version**: `1.0`  
> **Schema Contract**: [`schema/omnia.scenario.v1.json`](../schema/omnia.scenario.v1.json)  
> **File Extension**: `.omnia.json`

Omnia scenarios are machine-authored, strictly validated JSON documents defining the universal entity hierarchy, dynamic ACL attributes, physical spatial topology, and the 3-tier subjective memories (working buffer, archive episodes, and reality dossiers) for multi-agent simulation.

---

## 1. Document Structure & Schema Declaration

Every scenario document is a single self-contained JSON file (`*.omnia.json`). To enable autocompletion, field documentation, and live schema validation in editors like VS Code, declare the relative path to the schema using the `$schema` key:

```json
{
  "$schema": "../schema/omnia.scenario.v1.json",
  "format": {
    "name": "omnia.scenario",
    "version": "1.0"
  },
  "meta": {
    "id": "accidental-arrangement",
    "title": "Accidental Arrangement (Slice of Life)",
    "description": "A high-stakes domestic comedy in university student housing.",
    "tags": ["slice-of-life", "comedy", "university"],
    "created_at": "2026-10-16T18:00:00Z"
  },
  "characters": [ ... ],
  "entities": [ ... ],
  "history": [ ... ],
  "minds": [ ... ],
  "starts": [ ... ]
}
```

### Canonical Serialization Rules
1. **UTF-8 with LF line endings**.
2. **Normalized, not nested**: Entities, characters, and memories exist as flat arrays of records referenced by slug IDs.
3. **No `null` values**: Empty lists are `[]` and empty strings are `""`. Omitted fields default according to the schema.
4. **JS-safe numbers**: All integers must be within `[-2^53 + 1, 2^53 - 1]`.
5. **No unknown fields**: The loader enforces `DisallowUnknownFields`. Any authoring tool or GUI metadata must reside in the reserved `editor` object (`"editor": { ... }`), which is ignored by the engine.

---

## 2. ID Scoping Rules

Omnia enforces two distinct ID scopes:
1. **Collection Scope**:
   - `characters`: Unique slug identifier across all characters.
   - `entities`: Unique slug identifier across all physical spaces, portals, items, and actor bodies.
   - `starts`: Unique slug identifier across all start points.
   - `history`: Unique slug identifier across all history records.
2. **Mind Scope**:
   - `subjects`, `claims`, and `episodes` are scoped **strictly within their owner's mind**.
   - A subject ID in Zack's mind (`"nelly"`) and Nelly's mind (`"zack"`) are separate mental symbols that carry subjective beliefs and fallible bindings.

Slug IDs must match `^[a-z0-9][a-z0-9_-]{0,63}$`.

---

## 3. World Entities & Spatial Topology

The simulation world is composed of entities with specialized components:

### A. Spaces
Spaces represent internal volume. Root spaces have no parent (`"parent_id": ""`), while sub-spaces (such as pockets or compartments) attach to items or actors.
* `enterable`: `true` if human-scale actors can walk inside. Enterable spaces are interconnected via portals.
* `open`: `true` if physical ingress is unobstructed.
* `transparent`: `true` if optical perception penetrates room boundaries.
* `soundproof`: `true` if auditory sensory events do not penetrate.

### B. Portals
Portals connect two spaces:
* `from_id` / `to_id`: Origin and destination space entity IDs.
* `closed`: `true` if physical movement is obstructed (e.g. a closed door). Closed doors partition spaces into separate dynamic scenes.
* `locked`: `true` if physical opening is resisted.
* `transparent`: `true` if visual perception penetrates even when closed (e.g. glass sliding door).

### C. Universal Dynamic Attributes & ACL
Dynamic state attributes are attached to entities with fine-grained access control:
```json
{
  "key": "Penalty Clause",
  "value": "Failure to verify co-tenancy results in immediate lease revocation and $6,800 retroactive back-charges.",
  "visibility": "private",
  "accessors": ["nelly"]
}
```
* `visibility`: `"public"` (observable by any perceiver with line of sight) or `"private"` (restricted to Arbiter and entities in `accessors`).

---

## 4. 3-Tier Subjective Memory (`minds`)

Each character possesses a lifetime subjective memory record in `minds`:

### A. Subjects & Fallible Reality Desync
Subjects represent what exists in an actor's mental reality:
* `kind`: `"person"`, `"place"`, `"object"`, `"group"`, or `"idea"`.
* `origin`: `"authored"` or `"perceived"`.
* `bindings`: Believed correspondences to world entities (`"identity"`: *this IS that entity*; `"anchor"`: *located AT entity*).
* **Phantom Subjects**: Subjects with empty bindings (`"bindings": []`) represent absent people, abstract fears, or memories (e.g. Nelly's runaway ex `"david"` or the `"housing-authority"`).

### B. Subjective Reality Dossiers & Claims
Dossiers bundle what an actor believes, feels, or intends regarding a subject:
* `kind`: `"belief"`, `"feeling"`, `"suspicion"`, or `"intention"`.
* `salience`: Attention priority scale `0.05` to `1.0`.
* `conviction`: Certainty `0.0` to `1.0`.
* `fixed`: `true` protects core beliefs and delusions from temporal decay and Distiller mutation.
* `evidence`: Episode IDs in this mind supporting the claim.

### C. Archive Episodes & Beats
Condensed narrative memories of past lived or authored experiences:
* `precision`: `"minute"`, `"hour"`, `"day"`, `"month"`, or `"year"`.
* `place_ids` & `participant_ids`: Mental subject IDs where the event occurred and who was present.
* `importance`: Formative weight `0.05` to `1.0`.
* `beats`: Chronological chain of first-person condensed segments (`<= ~60 words`), each tagging mental `subject_ids`.

---

## 5. Starting Points & Overrides (`starts`)

A start point configures initial clock parameters, PRNG seed, player assignment, entity overrides, and start-specific mental state overlays:

```json
{
  "id": "outside-3b",
  "name": "Zack arrives at Bryn Hall outside Apt 3B",
  "clock": { "cycle": 1, "start_at": "2026-10-16T18:35:00Z", "tick_seconds": 20 },
  "player_id": "zack",
  "minds": [
    {
      "owner_id": "zack",
      "upsert_claims": [
        {
          "subject_id": "zack",
          "claim": {
            "id": "c-zk-self-1",
            "kind": "intention",
            "content": "Find out why Mia dragged me all the way across campus.",
            "salience": 0.95,
            "conviction": 0.85
          }
        }
      ]
    }
  ],
  "events": [
    {
      "id": "ev-zack-arrives",
      "tick": 1,
      "location_id": "breezeway-3f",
      "text": "You climb the concrete stairs to the third floor of Bryn Hall..."
    }
  ]
}
```

### Authoring Guidance for Events
1. **Describe Situation, Not Dialogue**: Opening events should set the physical scene and posture. Avoid pre-scripting speech (the validator issues a `LINT_SCRIPTED_DIALOGUE` warning for quoted text in event descriptions).
2. **Events Decide Who Acts First**: In Omnia, actors only deliberate when they receive sensory ingress. An event scheduled at a location wakes the actors present in that space and connected spaces.

---

## 6. CLI Validation

Validate any scenario document using the Omnia CLI:

```bash
# Human-readable validation report
omnia scenario validate scenarios/accidental_arrangement.omnia.json

# Machine-readable JSON output with JSON Pointers
omnia scenario validate scenarios/accidental_arrangement.omnia.json --json
```

All semantic issues carry precise JSON Pointers (`/minds/0/dossiers/1/claims/0/evidence/0`), error codes, and severities (`error`, `warning`, `info`).
