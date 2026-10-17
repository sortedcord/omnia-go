# Plan: Move Output Contracts from Prompts to Structured Outputs

## Goal

Every LLM call that returns JSON uses `response_format: {type: "json_schema", strict: true}`.
The output schema and the per-field descriptions move out of the system prompts and into the
schema. Prompts keep only the judgment rules: how to decide, never what shape to emit.

## Current State

| Call site | File | Format today | Schema + field docs in prompt |
|---|---|---|---|
| Actor deliberation | `pkg/actor/actor.go`, `prompts.go` (`DefaultActionSchema`) | `json_object` | Yes, a whole system message |
| Human intent interpreter | `pkg/sim/controller.go` (`interpret`) | `json_object` | Yes + classification rules |
| Scene Arbiter | `pkg/sim/arbiter.go` | `json_object` | Yes, about 30 lines + verdict/outcome/effect glossaries |
| Scene Broadcaster | `pkg/sim/broadcaster.go` | `json_object` | Yes |
| Thought Distiller | `pkg/sim/distiller.go` | `json_object` | Yes + thought-kind glossary |
| LOD Summarizer | `pkg/sim/lod.go` | plain text | No change needed |

`llm.ChatRequest.ResponseFormat` is a bare `string`, and `wireResponseFormat` only carries `type`.

## Probe Results (live endpoint, `gemini-3.5-flash-lite`)

| Feature | Result |
|---|---|
| `json_schema` + `strict: true` | Works |
| `enum` on strings | Works |
| Arrays of objects | Works |
| `description` on schema, properties, items | Works, and the model follows them |
| Map type (`additionalProperties: {type: string}`) | **Fails silently**: returned `{"perceptions": {}}` |

### Phase 0 results (verified with negative tests, `tools/probe_schema/`)

A "pass" counts only if the model was asked to break the constraint and the output still obeyed it.

| Feature | Enforced by grammar? | Evidence |
|---|---|---|
| `enum` (scalar and array items, 250 values) | **Yes** | Asked for `item-9999` → got `item-0001`; asked for Bob/Carol → `[]` |
| `required` keys | **Yes** | Asked to omit `nelly` → key present, but **value `""`** |
| `additionalProperties: false` | **Yes** | Asked for extra key → omitted |
| `type` (number vs string) | **Yes** | Asked for the word "many" → `0` |
| Nullable `["string","null"]`, `nullable: true`, `null` in enum | **Yes** | Emits `null` |
| `anyOf` of primitives | Yes | `{"v": 3}` |
| **`anyOf` of objects** | **No, silently dropped** | Got free-form strings and invented keys (`action`, `object`, `destination`) |
| **`minimum` / `maximum` / `maxItems`** | **No, ignored** | Asked for 5.0 with max 1.0 → `5.0`; maxItems 3 → 6 items |
| **Property order** | **Follows `required` order**, not `properties` order; `propertyOrdering` ignored | `required:[mid,zeta,alpha]` → output `mid,zeta,alpha` |
| Reasoning + strict schema | Yes | Reasoning comes back in `reasoning_details[type=reasoning.text]`; `pkg/llm` already parses it |
| Invalid schema | HTTP 400, body `INVALID_ARGUMENT` / `"Google upstream request failed"` | The body does **not** name `response_format` |
| Flat object + `type` enum + nullable fields | **Yes** | Clean `move` / `set_attribute` effects with correct nulls |

---

## Design Decisions

### D1. Typed `ResponseFormat` in `pkg/llm`

```go
type ResponseFormat struct {
    Type       string      // "json_object" | "json_schema"
    JSONSchema *JSONSchema // set when Type == "json_schema"
}

type JSONSchema struct {
    Name        string
    Description string
    Strict      bool
    Schema      *Schema
}

func JSONObject() *ResponseFormat
func StrictSchema(name, description string, s *Schema) *ResponseFormat
```

`ChatRequest.ResponseFormat` changes from `string` to `*ResponseFormat`. That breaks all six
call sites at compile time, so none can be missed.

### D2. Our own `Schema` type with ordered properties

Go marshals `map` keys alphabetically. Generation order follows schema property order, so a
map would make the Arbiter write `effects` before `execution_order` and `verdicts`, and the
Actor write `action` before `action_type`.

```go
type Schema struct {
    Type        []string   // ["string"] or ["string","null"]
    Description string
    Enum        []any      // may include nil when nullable
    Properties  []Property // ordered; custom MarshalJSON emits an object in this order
    Items       *Schema
    // No AnyOf / Minimum / Maximum / MaxItems: Phase 0 showed this gateway ignores them.
}

type Property struct {
    Name   string
    Schema *Schema
}
```

- `MarshalJSON` always emits `additionalProperties: false` and `required: [all property names]`
  on objects, because strict mode requires both.
- Emits `required` in the same order as `Properties`. **Phase 0: this gateway generates in
  `required` order**, and OpenAI follows `properties` order, so keeping both identical covers both.
  `propertyOrdering` is ignored here, so don't emit it.
- Builders keep schema code short: `Obj(desc, props...)`, `Str(desc)`, `Enum(desc, vals...)`,
  `Arr(desc, items)`, `Num(desc)`, `Nullable(s)`.
- `ValidateStrict(*Schema) error` walks the tree and rejects anything the provider won't
  enforce: maps, `anyOf` of objects, empty enums, nesting deeper than 5, and so on. It works
  from an allow-list proven in Phase 0: `type`, `enum`, `properties`, `required`,
  `additionalProperties`, `items`, `description`, nullable types. Unit tests run it on every
  schema we ship.

### D3. Optional fields become nullable

Strict mode makes every property required. Fields that are optional today become required and
nullable:

- `ThoughtOp.id`, `kind`, `salience`, `links`
- `Effect.parent_id`, `relation`, `closed`, `locked`, ...
- `IntentVerdict.remaining_intent`

Go decoding already handles `null`: it leaves zero values, and the pointer fields
(`Salience`, `Closed`, `Locked`, `Open`) stay `nil`. Domain structs barely change.

### D4. Split rule: what moves to the schema, what stays in the prompt

> **Schema**: what a field means, which values are legal, and what each enum value means.
> **Prompt**: how to decide, meaning policy, judgment, tone, and perspective.

| Text today | Destination |
|---|---|
| `"verdict": "as_intended" \| "altered" \| "preempted"` + glossary (Arbiter rule 2) | `verdict` enum + description |
| Outcome calibration glossary (rule 3) | `outcome` enum + description. Keep the policy part ("mishap is likelier with Luck ≤ 0.20 or stress") in the prompt |
| Effects whitelist (rule 5) | Each effect variant's description |
| "NEVER invent entity IDs" | Removed. Dynamic enums enforce it (D6) |
| Simultaneity & precedence (rule 1) | Prompt |
| Objective-only resolution (rule 4) | `resolution` description says *what* ("past-tense, third-person ground truth"). The prompt keeps *why* (the Broadcaster handles subjectivity) |
| Attribute ACL principle (rule 6) | Prompt. It is decision policy, and you asked for it to live in the prompts. `visibility` / `accessors` get one-line descriptions that point to it |
| Actor `speak` / `act` / `wait` glossary | `action_type` description |
| "Declare intent, not outcome" | `action` description |
| Thought-kind glossary (Distiller rule 2) | `kind` description |
| Salience range and meaning | `salience` description. Range clamped in Go (Phase 0: `minimum`/`maximum` not enforced) |
| Distiller graph discipline (rule 3: dedupe, drop stale, 0–4 ops) | Prompt. Cap at 4 enforced in Go (Phase 0: `maxItems` not enforced) |
| Human interpreter classification + "no god-moding" | Classification goes to the `action_type` description. No-god-moding and pronoun resolution stay in the prompt |
| Broadcaster second-person / no meta-leak rules | Prompt. The perception field description says "second-person, 1–3 sentences" |
| All "MUST respond ONLY with JSON" / "double quotes" boilerplate | Deleted |

### D5. Reshape outputs the grammar can't express

- **Broadcaster `perceptions`** (map → fixed-key object). Build the schema per request:
  `perceptions` becomes an object with **one required property per actor ID in the scene**.
  The grammar then guarantees full coverage, and the "missing perception for actor" error
  becomes unreachable. The Go wire struct stays `map[string]string`.
  **Caveat (Phase 0):** `required` guarantees the key, not content; the model returned `""`
  for a key it wanted to skip. The existing empty-string check for actors who acted stays.
- **Arbiter `verdicts`** (array → keyed object): `verdicts: {zack: {...}, nelly: {...}}`, with
  every actor required. That guarantees one verdict per actor, never duplicates. An adapter
  converts it to `[]IntentVerdict` in `ExecutionOrder` order.
- **Arbiter `effects`** use **one flat object** (decided by Phase 0: `anyOf` of objects is
  silently ignored by this gateway). Shape: `type` enum, `entity_id` enum, every other field
  nullable (with entity-ID enums + `null` where applicable). Verified working in probe N. Each
  field's description names which effect types use it, e.g. *"parent_id: target parent; move only,
  else null"*. Go-side `applySceneEffectsLocked` keeps rejecting incoherent combinations.
  `ValidateStrict` rejects `anyOf` of objects so it can't slip back in.
- **`set_condition`**: since the `Space.Conditions` refactor it is `set_attribute` on a space.
  Fold it into `set_attribute` and drop it from the schema. `EffectSetCondition` can stay as a
  Go alias for back-compat.

### D6. Dynamic enums: entity IDs enforced by the grammar

Schemas are built per request from scene context:

| Field | Enum source |
|---|---|
| Arbiter `execution_order[]`, verdict keys | IDs of actors in the scene |
| Arbiter `effects[].entity_id`, `parent_id` | Entity IDs rendered in `WorldState` |
| Arbiter `accessors[]` | All mind-bearing entity IDs |
| Broadcaster perception keys | Scene actor IDs |
| Distiller `links[]` | Perceivable + known entity IDs |
| Actor / interpreter `targets[]` | Perceivable entity **names** (actors target by name) |

Rules:

- An empty list falls back to a plain `Str`, because an empty enum is invalid.
- Over a cap (about 200 values, or the provider limit found in Phase 0), fall back to `Str`
  and keep Go-side validation.
- Go-side validation stays in every case, since grammars catch shape errors but not meaning
  errors (`applySceneEffectsLocked` checks and scene-boundary rules are unchanged).

Tradeoff: per-request schemas defeat provider-side grammar caching. Expect a small latency
cost on the first token, and measure it in Phase 5.

### D7. Actor system prompts

- Delete `PromptActionSchema` / `DefaultActionSchema()` from `DefaultSystemPrompts`. The
  uncommitted "JSON hardening" edit in `prompts.go` gets dropped along with it.
- Add `Actor.ResponseSchema func(stimulus SensoryEvent) *llm.ResponseFormat`, settable via
  `WithResponseSchema(...)`, default `ActionSchema(perceivableNames)`. Actors stay composable
  without a hidden prompt/schema coupling.
- `DefaultCognitionRule` keeps "never mention schemas/JSON in your thoughts". Reasoning stays
  free text, because the grammar only constrains `content`. The failure we saw while building
  the visualizer, where Nelly put her monologue in `content`, can no longer happen.

### D8. Decoding and fallback

- With `strict` schemas, decode via plain `json.Unmarshal`. If that fails, it's a provider
  bug: keep `DecodeJSON` as a last-resort path and log it.
- **Provider fallback**: if a request with `json_schema` returns **HTTP 400** (Phase 0: this
  gateway's body says only `INVALID_ARGUMENT` and never names `response_format`, so match on the
  status alone), the client:
  1. retries once with `json_object`,
  2. appends `Schema.PromptText()`, a compact rendering of the same schema and descriptions,
     as a trailing system message,
  3. records the capability on the `HTTPClient` so later calls skip the failed attempt.

  The schema stays the single source of truth even on degraded providers. Behind
  `llm.WithSchemaFallback(true)` (default on).

---

## Phases

Each phase is one commit. `go vet ./... && go test -race ./...` stays green after each.

### Phase 0: Capability probe (throwaway, `tools/probe_schema.go`, not committed)

Live-test against the endpoint:
1. `anyOf` union inside array items
2. `type: ["string","null"]` and `null` inside `enum`
3. Property ordering: does output order follow the schema, with and without `propertyOrdering`?
4. `minimum`/`maximum`/`maxItems`
5. Required-key object built from dynamic IDs (Broadcaster shape)
6. Dynamic enum with about 50 and about 300 values
7. `strict` + `reasoning_effort` + `include_reasoning` together: does reasoning still stream?
8. An error case: deliberately invalid schema → what HTTP status/body comes back (sets the D8
   detection rule)

The results decide the `anyOf` vs flat-effects fallback, nullable syntax, the enum cap, and
the ordering keyword.

### Phase 1: `pkg/llm` plumbing
- `schema.go`: `Schema`, `Property`, builders, ordered `MarshalJSON`, `ValidateStrict`, `PromptText`
- `types.go`: `ResponseFormat`, `JSONSchema`, `JSONObject()`, `StrictSchema()`
- `wire.go` / `client.go`: serialize `{type, json_schema: {name, description, strict, schema}}`;
  D8 fallback
- Call sites move to `llm.JSONObject()` with no behavior change
- Tests: marshal golden (order preserved, `required`/`additionalProperties` injected),
  `ValidateStrict` table tests, fallback path with an `httptest` server

### Phase 2: Actor + Human interpreter
- `pkg/actor/schemas.go`: `ActionSchema(targetNames []string)`
- Remove `DefaultActionSchema` from defaults; add `ResponseSchema` option
- `controller.go` `interpret()`: same schema; prompt shrinks to no-god-moding + pronoun
  resolution + phrasing
- Tests: request carries the schema (fake client); the actor system prompt no longer contains
  `action_type`

### Phase 3: Distiller
- `distillerSchema(linkIDs []string)`: `ops[]` with `op` enum, nullable `id`/`kind`/`salience`,
  `links` enum
- Prompt keeps only: subjectivity rules, ACL note, graph discipline
- Tests: decode with nulls → `ThoughtOp` with nil `Salience`; `ApplyOps` unchanged

### Phase 4: Broadcaster + Arbiter
- `broadcasterSchema(actorIDs)`: fixed-key perceptions object
- `arbiterSchema(actorIDs, entityIDs, mindIDs)`: ordered `execution_order` → `verdicts` (keyed)
  → `resolution` → `effects` (union or flat per Phase 0)
- Wire structs + adapters (`arbiterWire` → `SceneRuling`)
- Fold `set_condition` into `set_attribute` on spaces
- The Arbiter prompt drops rules 2, 3 (glossary part) and 5. It keeps 1, 4, the luck/mishap
  policy, and the ACL principle (6)
- Tests: adapter ordering, coverage guarantees, existing effect tests unchanged

### Phase 5: Measure and document
- Re-run `tools/generate_visualizer.go` (uncommitted) and compare against the current capture
  (12 calls, 22,642 tokens). Note: providers count the schema as prompt tokens, so savings come
  from removing *duplicated* text, not from the schema being free. Report honestly.
- Update `docs/arbiter.md` and `docs/scenes.md` (effect list, ACL, set_condition folding)
- Live runs: `go run main.go -play=false -ticks 3` and a curl playthrough via `cmd/omnia-http`

## Acceptance Criteria

1. No prompt string in `pkg/` contains a JSON skeleton, the words "respond ONLY with JSON", or
   an enum glossary. Checked with a grep test.
2. Every JSON-returning call sends `type: json_schema, strict: true`, verified via the tracer.
3. `ValidateStrict` passes on every shipped schema, for both empty and large dynamic contexts.
4. The Broadcaster "missing perception" error and the Arbiter missing-verdict case are
   impossible on schema-capable providers.
5. On a provider without `json_schema` support, the D8 fallback keeps the simulation running.
6. All existing tests pass; new tests cover schemas, adapters, and fallback.

## Risks

| Risk | Mitigation |
|---|---|
| Gateway (Antigravity → Gemini) translates only a subset of JSON Schema | Phase 0 decides; flat-object fallback for unions |
| Generation order ≠ schema order | Resolved: gateway follows `required` order; we emit `required` = property order |
| Gateway silently ignores unsupported keywords (`anyOf` objects, min/max, maxItems) instead of erroring | `ValidateStrict` allow-list of keywords proven by Phase 0; everything else rejected at build time |
| Fallback on 400 retries non-schema errors too | Retry once only; the retry must also succeed or the original error is returned |
| Dynamic enums blow provider schema limits on big worlds | Cap + `Str` fallback + Go validation |
| Grammar-constrained decoding lowers prose quality in long string fields | Compare Arbiter/Broadcaster prose side by side in the Phase 5 visualizer run |
| Per-request schemas hurt latency (no grammar cache) | Measure in Phase 5; cache schemas keyed by ID-set hash if needed |
