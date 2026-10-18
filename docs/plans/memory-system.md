# Plan: Three-Tier Actor Memory (Working Buffer · Episodes · Dossiers)

## 1. Why

The current actor memory (`pkg/actor/memory.go`) has two parts:

- `Turns`: verbatim perception, cognition and action. The last 10 are replayed, and everything older is **silently forgotten**.
- `Thoughts`: a flat graph of intentions, beliefs, feelings and suspicions with entity links. They decay at 0.92 per tick and are forgotten below 0.08.

That can't support the core of the product, which is people with lives:

1. **No long-term memory.** After 10 turns the past is gone.
2. **One decay for everything.** "My cheeks are burning" and "Mia always abandons me" fade at the same rate.
3. **No structure about others.** Nothing represents "what Nelly thinks of Zack".
4. **No way to author a past**, and no way for memory to **disagree with reality**. Today, beliefs link straight to canonical entity IDs, so a remembered house and the real house are the same thing.

## 2. The model

```
                ┌──────────────────────────────────────────────────────────────┐
  perceive/act  │ TIER 1  WORKING BUFFER     verbatim, high-res, right now      │
 ─────────────▶ │ perception · own cognition · own action   (per turn, ordered) │
                └───────────────┬──────────────────────────────────────────────┘
                                │ consolidate (scene ends / buffer over budget)
                                ▼
                ┌──────────────────────────────────────────────────────────────┐
                │ TIER 2  ARCHIVE            condensed, grouped by scene        │
                │ Episode = one scene, as THIS actor lived it (chain of beats)  │
                └───────────────▲──────────────────────────────────────────────┘
                     links      │ surface (cue-driven recall)
                ┌───────────────┴──────────────────────────────────────────────┐
                │ TIER 3  SUBJECTIVE REALITY                                    │
                │ Subject  = a thing that exists in THIS actor's mind           │
                │ Dossier  = what the actor thinks of that subject              │
                └───────────────▲──────────────────────────────────────────────┘
                                │ recognition through BINDINGS (beliefs, may be wrong/stale)
   ═════════════════════════════╪═══════════ PERCEPTION BOUNDARY ═══════════════
                                │ only what the actor perceives crosses
                         canonical world (sim entities)
```

**The data flow:** what's happening now lives verbatim in the buffer. When a scene ends, it condenses into an **episode**. Beliefs live in **dossiers**, one per **subject** in the actor's mind. When a subject becomes relevant, its dossier activates and its episode links **surface** the right memories.

**What changed:** dossiers are about *mental subjects*, not world entities. A subject may be **bound** to a world entity, but the binding is itself a belief that can be wrong, stale or missing. The world never writes to memory. This is what makes delusions, stale memories and reality drift possible (§6).

### Invariants

- **I1. Everything is subjective.** All tiers belong to one actor and are built only from what that actor perceived, thought or did. Arbiter resolutions, other actors' cognition and private attributes without accessors never enter.
- **I2. The world never writes to memory.** No engine code changes memory content because the world changed: no sync, no cleanup when an entity is destroyed, no fixing of false beliefs. Perception is the only input, and only through the Distiller and the Consolidator, plus the mechanical binding *confirmation* described in §6.
- **I3. Memory is invisible to the referee layer.** The Arbiter and the Broadcaster never read buffers, episodes or dossiers. If a belief must affect a physical outcome, it has to be an attribute.
- **I4. No LLM call on the read path.** Recall is deterministic scoring. LLM calls only happen when writing, in the background.
- **I5. Nothing is lost by failure.** A turn leaves the buffer only after its consolidation has been committed.
- **I6. Prompt size is bounded and recall is explainable.** Each tier has a token budget, and every surfaced item carries a score breakdown in `TickReport`.

---

## 3. Tier 1: Working Buffer

**What it holds:** the actor's own recent experience, verbatim. It replaces `Memory.Turns`.

```go
type Turn struct {
    Seq        int64      // monotonic per actor; consolidation watermark
    Tick       int
    Time       time.Time  // in-world
    SceneID    string     // episode boundary key
    Perception string     // exactly what the actor perceived
    Perceived  []Percept  // entity IDs perceived this turn + LOD + rendered name (input to recognition, §6)
    Cognition  string     // CleanCognition(reasoning); "" for human-controlled actors
    Action     ActionRecord
    EpisodeID  string
}
```

**Rules**
- **Budget.** The budget is in **tokens** (chars/4), 2,500 by default, and the last **4 turns are always verbatim**. This replaces `MaxHistoryTurns = 10`.
- **Prompt rendering.** Turns are replayed as user/assistant pairs. Cognition is stored but **not replayed**; feelings carry forward through the self-dossier.
- **Eviction.** Only after consolidation commits (I5). If the Consolidator keeps failing and the buffer reaches **2× budget**, a mechanical fallback writes the evicted turns as a beat made from each perception's first sentence and the action text, flagged `degraded`.
- **Missed perceptions.** Inbox perceptions that arrive while the actor is asleep keep the `SceneID` they occurred in.

## 4. Tier 2: Archive (Episodes)

**An episode is one scene, as one actor lived it.** It is a condensed chain of beats.

```go
type Episode struct {
    ID             string      // "ep-nelly-12": short, prompt-friendly
    OwnerID        string
    Source         Source      // lived | authored   (hearsay reserved; deferred)
    Status         Status      // open | closed
    SceneID        string      // "" for authored
    PrevID         string      // previous episode of this owner: chronological chain
    Start, End     time.Time
    Precision      Precision   // minute … year
    Label          string      // "the summer I turned twelve"
    PlaceIDs       []string    // SUBJECT IDs: where the actor believed they were
    ParticipantIDs []string    // SUBJECT IDs: who the actor believed was there
    PerceivedIDs   []string    // world entity IDs perceived (provenance only; never used for recall)
    Title          string      // ≤ 12 words
    Summary        string      // first-person, ≤ ~80 words
    Beats          []Beat
    Tone           string
    Importance     float64     // 0.05–1, set at close
    RecallCount      int
    LastRecalledTick int
    LastRecalledAt   time.Time
}

type Beat struct {
    FromSeq, ToSeq int64
    Text           string      // first-person condensed segment, ≤ ~60 words
    SubjectIDs     []string
    Degraded       bool
}
```

Episodes are recorded in **subject space**. Participants and places are the subjects the actor recognized at the time. If Nelly mistook a stranger for Leo, the episode says Leo was there, and that error is the point.

### Lifecycle (per actor)

| Event | Effect |
|---|---|
| The actor's first turn in a scene | **Open** an episode (its ID exists from the start, so claims can cite it) |
| The buffer exceeds its budget while the episode is open | Consolidate the oldest turns into **one beat**; evict them |
| The actor's `SceneID` changes, or the in-world gap since their last turn exceeds 10 min | **Close**: remaining turns become beats, then title, summary, tone, importance and participants are finalized |
| Room change within the same scene | **No split by default.** `SplitOnRoomChange` config flag, default `false`; it only appends to `PlaceIDs` |
| The simulation ends | Close every open episode |

Scene merges: an actor whose scene ID survives a merge keeps their episode, and an absorbed actor closes theirs and opens a new one. Content is **immutable after close**; reconsolidation is deferred.

## 5. Tier 3: Subjects and Dossiers

### Subjects: things that exist in one actor's mind

```go
type Subject struct {
    ID        string        // owner-scoped, "s-leo"
    Kind      SubjectKind   // person | place | object | group | idea
    Names     []string      // what THIS actor calls it ("Ne-Ne", "my ex", "Mom's house"); drives mention cues
    Bindings  []Binding     // believed correspondences to the world (may be empty, wrong or stale)
    Origin    Origin        // perceived | authored | told (told reserved with hearsay)
}

type Binding struct {
    EntityID          string      // a world entity ID; MAY NOT EXIST ANYMORE (or ever)
    Mode              BindingMode // identity: "this IS that" | anchor: "it is located at that"
    Confidence        float64     // 0–1; the actor's certainty, not truth
    LastConfirmedTick int         // last time perception agreed with the binding
    LastConfirmedAt   time.Time
}
```

- **The subject registry** replaces `KnownEntities`. "Known" now means *exists in my mind*, not "exists in the world".
- **Subjects with no binding are first-class:** the ex who left, deceased parents, the housing board, a god, an imaginary friend.
- **Bindings are never checked against the world.** A binding to an entity that was destroyed, or never existed, is valid. It just never fires through recognition. Anchor bindings still can fire (see §6, worked example).
- **Mechanical binding updates** are limited to: (a) **auto-register.** When the actor perceives an entity at LOD2 or higher that no subject is bound to, create a subject with an identity binding to it, using the *rendered* name as seen. (b) **Confirm.** When a bound entity is perceived, update `LastConfirmed*`. Nothing else changes bindings mechanically.

### Dossiers: what the actor thinks of a subject

```go
type Dossier struct {
    SubjectID  string       // includes the owner's own self-subject
    Impression string       // one-line gist, ≤ 25 words
    Claims     []Claim
    Links      []EpisodeLink
    Updated    int
}

type Claim struct {
    ID         string        // "c-nelly-zack-3"
    Kind       ClaimKind     // belief | feeling | suspicion | intention
    Content    string        // first-person
    Salience   float64       // how present it is in mind (0.05–1)
    Conviction float64       // how strongly it is held (0–1); input to the Distiller's judgment
    Fixed      bool          // author-set: the Distiller may NOT retract or rewrite it (delusion, core conviction)
    Evidence   []string      // episode IDs
    Formed, Updated int
    History    []Revision    // last 5 prior contents
}

type EpisodeLink struct {
    EpisodeID string
    Weight    float64        // 0–1 cue strength
    Role      LinkRole       // participant | place | evidence | formative | mentioned
}
```

### Rules
- **The self-dossier** holds feelings, intentions and self-image that aren't about another subject. Every current thought maps either to a subject's dossier or to the self-dossier.
- **Decay depends on the claim kind and in-world time:**

  | Kind | Half-life | Floor |
  |---|---|---|
  | feeling | 15 min | none |
  | intention | 2 h, while unresolved | none |
  | suspicion | 3 days | 0.10 |
  | belief | 30 days | 0.15 |

  Claims are never decayed below the floor. Claims below 0.08 with no floor are archived (hidden from context, kept for introspection). `Fixed` claims don't decay below 0.5.
- **Links are mechanical:**
  - Participant and place subjects of a closed episode get links (weight 0.5).
  - Claims formed or updated during an open episode get an evidence link (0.8).
  - The Consolidator can mark subjects `formative` (1.0).
  - Each dossier keeps its top 40 links by `weight × importance`.
- **Conviction and Fixed shape how belief changes.** The Distiller sees conviction and is instructed that held beliefs resist contrary evidence in proportion to it: it may add a doubting claim instead of rewriting. `Fixed` claims are enforced in code; ops targeting them are rejected and logged. Contrary claims can still be added, so the dissonance itself is represented.

## 6. Recognition and reality de-sync

This is the only place where the world reaches memory, and it goes through the actor's perception set, never through the world directly.

### Recognition (every tick, mechanical)

For each perceived entity (from `Turn.Perceived`), find subjects with an **identity** binding to it. These become **recognized** subjects. For the actor's **current location chain** (current space and its ancestors), find subjects with an **anchor** binding to any of those spaces. These become **anchored** subjects, meaning "things I believe are here".

**Expectation miss:** an anchored subject that also has an identity binding, but whose identity-bound entity is **not** in the perceived set, is *expected here but not seen*. That is computed only from belief plus perception, so it never peeks at the world. If the house is still there but at LOD0, the actor genuinely doesn't see it, and that's correct.

**The miss is a recall cue, never prompt text.** It raises the dossier's activation and so surfaces the linked episodes. Nothing in the prompt says "expected", "missing" or "not seen". The model gets the remembered place and the perceived place side by side and has to notice the gap itself. Two consequences:
- **No false alarms.** A pen believed to be in a closed drawer only gives the pen's dossier a slight lift. It is never announced as missing.
- **The reaction isn't guaranteed.** The probe in §12 checks how reliably the model notices, and that result tunes the miss activation and the vivid-memory threshold.

### Worked example: the demolished childhood home

Authored state for actor Ruth:

```
subject s-home  kind=place  names=["home", "the house on Elm"]
  bindings: identity → house-elm-14   (conf 0.95, confirmed 1998)
            anchor   → lot-elm-14     (conf 0.95)
dossier s-home:
  impression: "The blue two-story house with the porch swing where Mom read to me."
  claims: belief  "My childhood home still stands on Elm Street."  conviction 0.9
          feeling "That porch is the safest place I've ever known." 
  links:  ep-ruth-03 (formative), ep-ruth-07
```

Current world: `house-elm-14` no longer exists. `lot-elm-14` now contains `office-tower-2`.

1. Ruth walks into `lot-elm-14`. Her location chain includes it, so `s-home` is **anchored**.
2. Identity binding `house-elm-14` is not in her perceived set, so **expectation miss**. Activation of `s-home` is 0.9, and `formative` links surface `ep-ruth-03` vividly.
3. **The prompt**, in Ruth's own words:
   - **What she perceives:** *"a glass office tower, a security turnstile."*
   - **What she thinks:** *"home: the blue two-story house with the porch swing…"* (no marker; the dossier is simply highly activated)
   - **What she remembers:** *"the summer I turned twelve, reading on the porch swing…"*
4. The actor reacts, if the model notices the gap between memory and perception. Possible reactions are denial, grief, or asking a guard.
5. The Distiller may unbind `identity → house-elm-14`, add a feeling, or revise the belief. With conviction 0.9 it more likely adds a suspicion ("they must have moved it, or I'm on the wrong street"). With `Fixed`, the belief persists against all evidence, which is a delusion.

The engine never says "the house was demolished." The world is *perceived*, memory is *believed*, and the actor sits between the two.

### Desync categories this supports

| Category | How it arises |
|---|---|
| Stale memory | A binding to a changed or destroyed entity; old claims with floors |
| Delusion | `Fixed` false claims; a high-conviction belief the Distiller won't overturn |
| Misidentification | Authored or Distiller identity binding to the wrong entity, e.g. believing a stranger is your son |
| Phantom subjects | Subjects with no binding: dead relatives, imaginary friends |
| Unrecognized change | An entity is replaced but keeps its ID (authored world edit), so the claims are now false |

**Known v1 limit:** recognition through identity bindings is **perfect**. A known person in a disguise is still recognized by entity ID. Fallible recognition, based on how an entity *presents* to the perceiver, is deferred. Until then, misrecognition can only be authored or come from the Distiller.

## 7. Recall: how dossiers surface episodes

Recall runs every tick with no LLM call, entirely in subject space.

### Step 1: activate dossiers

| Cue | Activation |
|---|---|
| Recognized subject at LOD3 / LOD2 / LOD1 | 1.0 / 0.6 / 0.3 |
| Named in this tick's perception (subject's `Names`) | 0.9 |
| Anchored subject (believed to be here) | 0.5 |
| Expectation miss | 0.9 |
| Focus map (via recognized entity) | + focus boost (cap 1.0) |
| Self-dossier | 0.4, always |

`dossierScore = activation × (0.5 + 0.5 × max claim salience)`. The self-dossier plus the **top 4** others are selected.

### Step 2: score linked episodes

```
cue(e)     = Σ selected dossiers d linking e:  activation(d) × link.weight
access(e)  = max( (1 + Δhours/24)^-0.5 × (1 + 0.15·ln(1+recallCount)),  0.35 if importance ≥ 0.8 else 0 )
context(e) = 0.5·Jaccard(e.participants, recognized subjects) + 0.5·[e.places ∩ (anchored ∪ current place subject) ≠ ∅]
refract(e) = 0.5 if surfaced in the last 3 ticks with no new mention or expectation-miss cue, else 0
score(e)   = cue × (0.4 + 0.6·importance) × access + 0.2·context − refract
```

The open episode is excluded. Episodes scoring at least **θ = 0.25** are kept, up to the top 3, within budget. A score of 0.75 or more renders the beats (a vivid memory). Otherwise the title, when, where and summary are shown. Surfacing updates the recall bookkeeping. All constants live in a `RecallConfig`, and scoring is pure, so golden tests are possible.

### Step 3: assemble the prompt

| # | Role | Content | Budget |
|---|---|---|---|
| 1 | system | Grounding, cognition rule | fixed |
| 2 | developer | Profile | fixed |
| 3 | developer | Accessible attributes | fixed |
| 4 | developer | **Self**: impression, active feelings and intentions | 300 |
| 5 | developer | **What you think of…**: activated dossiers (no expectation-miss markers; the miss only affects activation) | 700 |
| 6 | developer | **Memories surfacing** | 900 |
| 7 | user/assistant | Working buffer | 2,500 |
| 8 | user | Current stimulus | n/a |

## 8. Write path

| Writer | When | Output (structured-outputs schema) |
|---|---|---|
| **Buffer append** (engine) | Every turn | `Turn` + mechanical recognition (auto-register, confirm) |
| **Distiller**, retargeted | Every turn, in the background | `ops[]`, listed below |
| **Consolidator** (new) | Buffer over budget / episode close | `beats[]`; on close: `title`, `summary`, `tone`, `importance`, `formative_subject_ids[]` |

The Distiller's `ops[]` are:
- `upsert_claim{subject_id, claim_id?, kind, content, salience, conviction}`
- `retract_claim{claim_id}`
- `set_impression{subject_id, text}`
- `add_name{subject_id, name}`
- `new_subject{kind, name}`: a subject with no binding, e.g. someone just mentioned
- `bind{subject_id, entity_id, mode}`: entity enum restricted to *currently perceived* entities, e.g. "the hooded man is Leo"
- `unbind{subject_id, entity_id}`

Further rules:
- **Enums.** `subject_id` enums are the owner's subjects; `entity_id` enums are only the current perceived set. The Distiller can't bind to anything the actor can't see.
- **Fixed claims.** Ops on `Fixed` claims are rejected in code.
- **Watermark.** Consolidation is idempotent per `[FromSeq, ToSeq]`, with one job per actor at a time.
- **Ownership.** The Distiller owns subjects, claims and bindings. The Consolidator owns episodes. Links are mechanical, so the two writers never conflict.
- **Cost.** About one Consolidator call per actor per scene, plus one per buffer overflow (flash-lite, low reasoning), measured in the visualizer.

## 9. Authoring and serialization

All tiers serialize to JSON with stable IDs and counters, for scenario seeding now and save games later. A scenario can author:
- **Subjects** with names and bindings, including bindings to entities **absent from the start point or from the world entirely**.
- **Dossiers** with impressions and claims carrying `conviction` and `fixed`, linked to authored episodes.
- **Episodes** with `source: authored`, fuzzy dates (`precision`, `label`), beats and importance.

**Consequence for the deferred scenario-loader plan:** the validator must check binding *syntax* only. Referential integrity is enforced for structure (episode links, claim evidence, subject IDs inside a mind) but **never** between a mind and the world. A binding to `house-elm-14` in a start where it doesn't exist is the feature, not an error. At most it is an info-level note in the GUI ("bound to an entity not present in this start").

This replaces the `memories` / `reflections` records in that plan (now active: `docs/plans/scenario-loader.md`, `minds` records).

## 10. Package layout

`pkg/memory` is new and **does not depend on `sim`**; it works with plain string IDs.

| File | Contents |
|---|---|
| `buffer.go` | `Turn`, `Percept`, budgeted buffer, watermark |
| `episode.go` | `Episode`, `Beat`, lifecycle |
| `subject.go` | `Subject`, `Binding`, registry, recognition (identity, anchor, expectation miss) |
| `dossier.go` | `Dossier`, `Claim`, decay, ops application, the `Fixed` guard, mechanical links |
| `recall.go` | Activation, scoring, `RecallConfig`, `Recall(cues) RecallResult` with breakdowns |
| `render.go` | Prompt sections within budgets |
| `codec.go` | JSON snapshot |
| `mind.go` | `Mind`, which owns all tiers behind one mutex |

`actor.Actor` swaps `*Memory` for `*memory.Mind`. `pkg/sim` passes `Percept`s (entity ID, LOD, rendered name), the location chain, perception text and the scene ID into the mind at the existing points in `Step`. It never passes entity state.

## 11. Migration (hard cutover)

| Today | Becomes |
|---|---|
| `Memory.Turns`, `MaxHistoryTurns` | Working buffer with a token budget |
| `Thought` / `ThoughtOp` / `ApplyThoughtOps` | `Claim` / dossier ops / `ApplyDossierOps` |
| `FormatThoughtContext` / `SelectThoughts` | `render.go` sections 4–6 |
| `KnownEntities` / `LearnEntities` | Subject registry with auto-register on perception |
| `AddThought` in `pkg/scenario` | `AddSubject` / `AddClaim` |
| `TickReport.Thoughts` | `TickReport.Recall` (activations, surfaced episodes, breakdowns, expectation misses) |

## 12. Phases

Each phase is one commit, with `go vet` and `go test -race` green.

1. **`pkg/memory` skeleton and the working buffer.** Seq, scene, episode, percepts and the token budget.
2. **Subjects and dossiers.** Registry, bindings, mechanical recognition, self-dossier, kind-specific decay, conviction and `Fixed`. Retarget the Distiller (ops, schema, prompt). Rendering covers sections 4–5. Live run.
3. **Episodes and the Consolidator.** Lifecycle (scene and gap boundaries, `SplitOnRoomChange` off), beats, watermark, degraded fallback, mechanical links.
4. **Recall.** Activation cues (including anchors and expectation miss), scoring, memory detail levels, refractory period, budgets, `TickReport.Recall`.
5. **Serialization and seeding API.** Authored subjects, bindings, dossiers and episodes. This unblocks the scenario loader.
6. **Evaluation.**
   - A fake-LLM 60-tick, 4-scene run testing budgets, boundaries, links and failure injection.
   - The **desync probe** (live): the demolished-home setup from §6. Assert the expectation miss fires in `TickReport.Recall`, `ep-…-03` surfaces, the prompt contains **no** miss marker, and a `Fixed` belief survives the Distiller. Whether the action reacts to the discrepancy is **measured, not asserted**: report the reaction rate over N≥10 seeded runs and tune the miss activation (default 0.9) and the vivid threshold (default 0.8) from it.
   - A recall probe: a mention of an absent, unbound subject surfaces its episode.

## 13. Acceptance criteria

1. Prompt budgets hold on every tick of the 60-tick run (I6).
2. With every second Consolidator call failing, every `Seq` ends up in exactly one beat (I5).
3. Recall is deterministic: golden tables pass (I4, I6).
4. Static tests show that `pkg/memory` doesn't import `pkg/sim`, and that the Arbiter and Broadcaster code paths never reference `memory` (I2, I3).
5. A destroyed entity's bindings and claims are byte-identical before and after it is removed from the world (I2).
6. The desync probe and the recall probe pass. The desync reaction rate is reported, and it is a tuning target, not a gate.

## 14. Decisions

| # | Decision |
|---|---|
| Q1 | Non-relational feelings and intentions live in the **self-dossier** |
| Q2 | Boundaries are a scene change or a 10-min in-world gap. Room change within a scene **does not split by default** (`SplitOnRoomChange=false`) |
| Q3 | Reconsolidation **deferred**; episodes are immutable after close |
| Q4 | Subjects are **mind-owned and not canonically tied to the world**. Bindings are fallible beliefs, enabling stale memories, delusions and reality drift (§5–6) |
| Q5 | Hearsay **deferred**; `Source: hearsay` and `Origin: told` are reserved |
| Q6 | The Arbiter, and the Broadcaster, **never read memory** (I3) |
| R1 | Auto-register **everything the actor perceives** (LOD2 or higher) as a subject |
| R2 | The expectation miss is **implicit**: it only raises activation, and no marker is rendered in the prompt |
| R3 | Fallible recognition (disguise, aging, darkness) **deferred**; no follow-up plan yet |

