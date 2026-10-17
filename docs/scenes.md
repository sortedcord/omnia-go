# Scene-Based Multi-Agent Orchestration Architecture

## 1. Overview & Problem Statement

In complex multi-agent simulations, resolving the entire universe in a single global arbiter call ("everything everywhere all at once") causes severe architectural problems:
* **Context Pollution**: Characters in private rooms or separate tables have their local actions mixed into global prompts.
* **Unrealistic Simultaneity**: Unrelated actions across distant locations are forced into an artificial global ordering.
* **Cost & Scalability**: Global prompts grow linearly with every entity and room in the world.

To solve this, Omnia partitions the simulation into dynamic **Scenes**.

```
┌────────────────────────────────────────────────────────┐
│                   GLOBAL TIMELINE                      │
│  One Clock, Shared Tick, Unified Event Log             │
└───────────────────────────┬────────────────────────────┘
                            │ Partitions into
              ┌─────────────┴─────────────┐
              ▼                           ▼
┌───────────────────────────┐ ┌───────────────────────────┐
│     SCENE A (Living Room) │ │   SCENE B (Bathroom)      │
│  • Zack, Nelly            │ │  • Mia (behind shut door) │
│  • Scene-local Arbiter    │ │  • Scene-local Arbiter    │
│  • Scene-local Broadcaster│ │  • Scene-local Broadcaster│
└───────────────────────────┘ └───────────────────────────┘
              ▲                           ▲
              │   Conflict & Merge Rule   │
              └───────────────────────────┘
         (Contested boundary merges scenes next tick)
```

---

## 2. Core Concepts

### A. The Action Model & Targets
Every actor action includes an explicit `targets` field:
```json
{
  "action_type": "speak",
  "action": "Notes? Is there going to be a pop quiz?",
  "targets": ["Nelly"]
}
```
* **Intentional Addressee**: `targets` represents *who the actor intends to address or interact with*.
* **No Perception Gating**: Targets do not restrict who perceives the action. Anyone co-present in the scene overhears or witnesses the action.
* **Name Resolution**: The simulation matches target names to perceivable entity IDs. Unknown names are discarded.

### B. Mutable Attributes (Replacing `Status`)
Character physiology and state are managed through structured `Attributes`:
```go
type Attribute struct {
    Value   string
    Allowed []string // Non-empty indicates an enum (e.g. ["low", "normal", "high"])
    Core    bool     // Core attributes can be mutated, but cannot be removed
}
```
* **Core Attributes**: `mood` (free text) and `arousal` (enum: `low`, `normal`, `high`).
* **Scenario Attributes**: Configurable per simulation (e.g. `Social Battery`, `Oxygen Reserve`).
* **Mutations**: Arbiter commands `set_attribute` (validated against `Allowed`) and `remove_attribute` (rejected for core attributes).

### C. Dynamic Scene Partitioning
A **Scene** is a group of actors who can mutually perceive each other well enough to interact in real time:
1. **Connectivity**: Actors are connected if they occupy the same space or are joined by an open/transparent portal.
2. **Recomputation**: Scenes are dynamically recomputed at the start of each tick from the entity tree topology.
3. **Splits & Merges**:
   * *Split*: If Mia enters the bathroom and shuts the door, the topology splits into two independent scenes. The larger group keeps the existing `SceneID`; the smaller group receives a new ID. (Ties are broken by member longevity in the scene).
   * *Merge*: If Mia opens the door, the connected component rejoins into a single scene. The older `SceneID` (based on `CreatedTick`) survives.
4. **Quiet Scenes**: Scenes where no actor is awake skip Arbiter and Broadcaster calls entirely, consuming zero compute.

### D. Inter-Scene Conflict & Merging Rule
If actors in different scenes attempt to mutate the same shared boundary entity (e.g. a portal door) in the same tick:
1. **Precedence**: Effects are executed in order of scene age (`CreatedTick`). The older scene's effects take precedence; conflicting effects from the newer scene are rejected and logged.
2. **Temporary Bond**: The newer scene is absorbed into the older scene for the next tick, forming a single combined scene so one Arbiter adjudicates the contest directly.
3. **Dissolution**: The bond persists as long as actors on both sides continue to interact with the contested entity, and dissolves on the first tick neither side acts upon it.

---

## 3. The Tick Pipeline

```
1. Build Scenes     Derives scenes from spatial topology + active conflict bonds.
2. Deliberate       Woken actors across all scenes deliberate in parallel (goroutines).
                    Human controllers block only their own scene's progress while LLMs run.
3. Resolve Targets  Matches declared target names to perceivable entity IDs.
4. Score            Computes deterministic spontaneity scores for all candidate actions.
5. Adjudicate       One Arbiter call per active scene (running in parallel).
                    Produces: verdicts (as_intended, altered, preempted), execution order,
                    objective resolution, remaining intent, and world mutations.
6. Apply Effects    Applies effects in scene age order; detects and resolves boundary conflicts.
7. Broadcast        One Broadcaster call per scene in parallel.
                    Translates objective resolution into individualized second-person perceptions
                    (e.g. what an actor felt/heard vs. what onlookers noticed vs. background murmurs).
8. Deliver & Wake   Perceptions are delivered to inboxes. Anyone with a non-empty perception wakes.
                    Actors who acted always wake (their perception includes their own outcome).
9. Distill          Background Thought Distillers run concurrently for actors who took turns.
```

---

## 4. Spontaneity Scoring

To break conflicts fairly without relying on artificial alphabetical ordering:

$$\text{Score} = 0.25 \cdot \text{Targeted} + 0.30 \cdot \text{Attention} + 0.20 \cdot \text{Readiness} + 0.10 \cdot \text{Proximity} + 0.15 \cdot \text{Jitter}$$

* **Targeted**: `1.0` if this actor was directly addressed by name in recent scene events; `0.0` otherwise.
* **Attention**: Dynamic focus level from the actor's `FocusMap` on the event source (normalized `0.0` to `1.0`).
* **Readiness**: Derived from `arousal` attribute: `low` = `0.20`, `normal` = `0.50`, `high` = `0.85`.
* **Proximity**: `1.0` for same space, `0.5` across open portal, `0.0` otherwise.
* **Jitter**: Deterministic hash: $\text{hash}(\text{Seed}, \text{Tick}, \text{ActorID}) \pmod{1000} / 1000.0$. Guarantees reproducible scores regardless of parallel execution order.

*Advisory Nature*: Spontaneity scores provide guidance to the Arbiter for settling simultaneous actions and interruptions; they never strip an actor's turn.

---

## 5. Component Responsibilities

| Component | Visibility | Output |
| :--- | :--- | :--- |
| **Actor Mind** | Subjective, LOD-filtered surroundings, gear, private thoughts | Declared intent + targets (by name) |
| **Arbiter** (per scene) | Objective scene ground truth, actor attributes, luck, spontaneity, global event digest | Canonical resolution, verdicts, remaining intent, structured mutations |
| **Broadcaster** (per scene) | Objective resolution, actor positions, attention states (NO private thoughts) | Individualized subjective perceptions for all actors in the scene |
| **Thought Distiller** (background) | Subjective stimulus, raw internal monologue, declared intent | Atomic mutations on actor's private thought graph (`add`, `update`, `drop`) |
