# The Arbiter Architecture (Intent vs. Execution)

## 1. Problem Statement: The "Runaway Narrator"

In monolithic narrative AI games (such as AI Dungeon), a single model plays the Game Master, the world physics, all NPCs, and frequently even the player character. This produces two critical design failures:

1. **Loss of Player Agency (The Runaway Narrator)**: The model conflates *intent*, *attempt*, *reaction*, and *final outcome* into a single generation (e.g. an NPC draws a weapon, speaks three sentences, crosses the room, attacks, and sets the room on fire without a single interrupt window for the player).
2. **God-Moding & Infallibility**: Actors dictate their own success, eliminating genuine physical mistakes, fumbles, or realistic constraints.

---

## 2. The 2-Tier Architecture

To preserve true interruptibility, agency, and physical realism, the simulation decouples **Subjective Intent** from **Objective Execution**:

```
┌────────────────────────────────────────────────────────┐
│                        ACTOR                           │
│  "I wake up from bed and walk to the kitchen for       │
│   a glass of water."                                   │
│  • Operates on subjective knowledge & beliefs          │
│  • Pinned Trajectory of what they intend to do         │
└───────────────────────────┬────────────────────────────┘
                            │ Declared Intent
                            ▼
┌────────────────────────────────────────────────────────┐
│                     THE ARBITER                        │
│             (Executioner / World Physics Engine)       │
│                                                        │
│  Inputs:                                               │
│  • Actor's Declared Intent                             │
│  • Actor's Physical Status (fatigue, adrenaline, gear) │
│  • Objective World State (lighting, hazards, layout)   │
│  • Other Co-Present Entities & Their Actions           │
│                                                        │
│  Resolution:                                           │
│  "She swings her legs out of bed in the dark, takes    │
│   two steps, catches her foot on a severed cable, and  │
│   stumbles hard into the doorframe with a loud thud."  │
└───────────────────────────┬────────────────────────────┘
                            │
            ┌───────────────┴───────────────┐
            ▼                               ▼
    Actor Sensory Feed              Onlooker Sensory Feed
 "You stub your toe on a         "You hear a loud thud and
  loose cable and slam into       a muffled curse from Room 4."
  the doorframe."
```

---

## 3. Core Principles

### A. Intent vs. Outcome
Actors are strictly forbidden from authoring final outcomes. An actor can only emit what they **attempt** to do:
* **Disallowed (Outcome)**: `"I shoot the rogue between the eyes and kill him."`
* **Allowed (Intent)**: `act: "I draw my service weapon, aim at the rogue's chest, and squeeze the trigger."`

The Arbiter determines whether the gun fires, kicks, misses, grazes cover, or if the target ducks behind a crate in time.

### B. Pinned Trajectories & Interruptibility
An actor can declare a macro-trajectory (e.g., *"Walk from the greenhouse to the command bridge"*). Rather than fast-forwarding the entire journey, the Arbiter resolves the path **up to the first inflection point**:
1. **Beat 1**: Actor exits the greenhouse into Corridor C.
2. **Beat 2**: In Corridor C, another actor is standing with a drawn weapon.
3. **Trajectory Halted**: The Arbiter pauses the trajectory immediately.
   * Actor A receives: *"You step into Corridor C and freeze. Officer Drake is ten paces away, weapon drawn."*
   * Officer Drake receives: *"The greenhouse bulkhead opens and Dr. Vance steps into the corridor."*
   * Both participants now enter a reaction/interrupt window.

### C. Genuine Mistakes & Physical Mishaps
Because the Arbiter evaluates the actor's `Status` (weariness, heart rate, adrenaline, injuries) against the `WorldState` (lighting, moisture, terrain, noise):
* An **exhausted, barefoot** actor walking in pitch darkness might stub their toe, trip on loose wiring, or misjudge a door handle.
* A **panicked, high-adrenaline** actor might fumble a keycard, drop an emergency patch kit, or strip a manual bolt.
* The actor *believed* they were performing a clean, calculated action; the Arbiter grounds it in physical reality.

---

## 4. Arbiter Data Contracts

### Inputs
1. **`ActorIntent`**:
   * `ActorID`: Unique identifier.
   * `ActionType`: `speak`, `act`, or `wait`.
   * `ActionDescription`: What the actor is attempting.
2. **`ActorStatus`**:
   * Vitals, fatigue level, emotional state, carried equipment.
3. **`WorldState`**:
   * Current room/zone conditions (lighting, atmosphere, noise, hazards).
   * Presence and active stances of all nearby entities.

### Outputs
1. **`OutcomeType`**:
   * `FullSuccess`: Action completed exactly as attempted.
   * `PartialSuccess`: Action completed with a complication or reduced effect.
   * `Failure`: Action failed without catastrophic mishap.
   * `Mishap`: Action resulted in an unintended mistake, fumble, or self-inflicted hazard.
   * `Interrupted`: Action was halted mid-stride by an environmental event or another actor's intervention.
2. **`ObjectiveResolution`**:
   * The canonical ground truth of what physically happened in the simulation world.
3. **`SubjectiveSensoryFeeds`**:
   * `ActorPerception`: What the acting character felt, saw, and heard.
   * `AuditoryPerception`: Sound waves broadcast to nearby rooms/zones.
   * `VisualPerception`: What onlookers in the direct line of sight witnessed.

---

## 5. Implementation (v1)

* **Core Engine**: Implemented in `pkg/sim/arbiter.go` and `pkg/sim/effects.go`.
* **The Referee Loop**: `Simulation.Step` passes all `act` actions to `Arbiter.Resolve`, applying validated world mutations before fanning out sensory feeds.
* **Whitelisted Structured Effects**:
  * `move`: Reparents entities within the spatial tree, enforcing reachability and portal traversal.
  * `set_condition`: Updates or removes environmental conditions on rooms/spaces.
  * `set_portal`: Adjusts `closed` and `locked` states on doors, hatches, and bulkheads.
  * `set_vital`: Modifies an actor's internal vitals (`Oxygen Reserve`, `Heart Rate`, etc.).
  * `set_open`: Opens or closes container spaces (e.g. vehicle trunks, cabinets).
  * `reveal`: Clears concealment on uncovered or searched items.
* **Actor Outcome Feedback**: The actor's inbox immediately receives their personal second-person sensory perception (`ActorPerception`), waking them on the following tick to observe what actually happened.
* **Seeded Luck Dice Roll**: An impartial float (0.00 to 1.00) is rolled by the engine and fed to the LLM Arbiter to prevent default-success bias and make genuine mishaps possible.
