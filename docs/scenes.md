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

### B. Mutable Attributes on Entities with ACL & Visibility
Every entity in the simulation (characters, props, vehicles, viewports, doors) owns an `Attributes` map with granular Access Control Lists (ACL):

```go
type Visibility string

const (
    VisibilityPublic  Visibility = "public"
    VisibilityPrivate Visibility = "private"
)

type Attribute struct {
    Value      string     `json:"value"`
    Allowed    []string   `json:"allowed,omitempty"`   // Non-empty = enum
    Core       bool       `json:"core,omitempty"`      // Cannot be removed
    Visibility Visibility `json:"visibility"`          // "public" or "private"
    Accessors  []EntityID `json:"accessors,omitempty"` // Whitelist of entity IDs who can observe this when private
}
```

* **The Core Attribute ACL Principle**:
  * **`public`**: The attribute is openly observable by anyone in the room (e.g. visibly trembling, a shattered window, room lights flickering). Shapes everyone's thinking. (`accessors` is automatically cleared out).
  * **`private` with Entity in Accessors** (e.g. `accessors: ["zack"]`): Set when you want an internal conscious state (e.g. racing heart, private sarcasm, conscious panic) to **directly affect that entity's thinking** and future choices.
  * **`private` with NO Accessors** (empty `accessors: []`): Set when you do **NOT** want the entity to inherently know about it, but that attribute **does affect the actual actions that are ultimately decided or done by the Arbiter**! (e.g. latent muscle fatigue, an unseen micro-fracture, subconscious panic impairing motor precision). The entity remains oblivious in their conscious thoughts, but the Arbiter uses this hidden attribute to realistically alter, restrict, or modify actions instead of intents directly translating into reality!
* **Mutations**: The Arbiter commands `set_attribute` (can mutate value, toggle public/private, and modify the accessor list) and `remove_attribute` (rejected for core attributes). When changing an attribute to public, the accessors list is cleared. When private, the Arbiter can mutate the accessors list directly.

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
3. **Dissolution**: The bond guarantees one merged tick. It is renewed while actors on both sides keep targeting the contested entity, and dissolves on the first tick they don't.
4. **Scope**: Only portals can be contested; each scene may only mutate entities inside its own spaces or on its boundary.

### E. Structured Outputs Contracts
All LLM components return data constrained at the sampler level via OpenAI-compatible Structured Outputs (`response_format: {type: "json_schema", strict: true}`):
* **Actor Action**: `action_type` (enum: `speak`, `act`, `wait`), `action` (dialogue/attempt), `targets` (constrained to perceivable names when available).
* **Arbiter Ruling**: `execution_order` (ordered IDs), `verdicts` (keyed by actor ID, guaranteeing full scene coverage), `resolution` (objective third-person ground truth), `effects` (flat nullable object with mutation enums).
* **Broadcaster**: `perceptions` (keyed by actor ID, guaranteeing 100% coverage for all present characters).
* **Thought Distiller**: `ops` (array of atomic operations with `op` and `kind` enums, links, and salience).
* **Prompt Cleanliness**: Zero JSON skeletons, zero schema formatting instructions, and zero output boilerplate in system prompts. Field definitions and enum meanings are carried directly on schema property descriptions.

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

* **Targeted**: `1.0` if another actor's resolved targets included this actor last tick; `0.0` otherwise. (This tick's targets are unknown while everyone deliberates in parallel.)
* **Attention**: Dynamic focus level from the actor's `FocusMap` on the event source (normalized `0.0` to `1.0`).
* **Readiness**: Derived from `arousal` attribute: `low` = `0.20`, `normal` = `0.50`, `high` = `0.85`.
* **Proximity**: `1.0` for same space, `0.5` across open portal, `0.0` otherwise.
* **Jitter** and **Luck**: FNV-1a hash of (seed, tick, actor, salt), with separate salts. Reproducible regardless of goroutine order or how many other actors act.

*Advisory Nature*: Spontaneity scores provide guidance to the Arbiter for settling simultaneous actions and interruptions; they never strip an actor's turn.

---

## 5. Component Responsibilities

| Component | Visibility | Output |
| :--- | :--- | :--- |
| **Actor Mind** | Subjective, LOD-filtered surroundings, gear, private thoughts | Declared intent + targets (by name) |
| **Arbiter** (per scene) | Objective scene ground truth, actor attributes, luck, spontaneity, global event digest | Canonical resolution, verdicts, remaining intent, structured mutations |
| **Broadcaster** (per scene) | Objective resolution, actor positions, attention states (NO private thoughts) | Individualized subjective perceptions for all actors in the scene |
| **Thought Distiller** (background) | Subjective stimulus, raw internal monologue, declared intent | Atomic mutations on actor's private thought graph (`add`, `update`, `drop`) |
