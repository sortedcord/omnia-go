package sim

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"omnia.adityagupta.dev/pkg/actor"
	"omnia.adityagupta.dev/pkg/llm"
)

// EffectType represents a valid mutation operator on the world state.
type EffectType string

const (
	EffectMove            EffectType = "move"             // Reparents an entity: entity_id, parent_id, relation, placement
	EffectSetCondition    EffectType = "set_condition"    // Updates space condition: entity_id, key, value
	EffectSetPortal       EffectType = "set_portal"       // Updates portal state: entity_id, closed, locked
	EffectSetAttribute    EffectType = "set_attribute"    // Updates actor attribute: entity_id, key, value
	EffectRemoveAttribute EffectType = "remove_attribute" // Removes non-core attribute: entity_id, key
	EffectSetOpen         EffectType = "set_open"         // Updates container openness: entity_id, open
	EffectReveal          EffectType = "reveal"           // Clears concealment on an entity: entity_id
)

// Effect is an atomic world-state mutation commanded by the Arbiter.
type Effect struct {
	Type       EffectType `json:"type"`
	EntityID   EntityID   `json:"entity_id,omitempty"`
	ParentID   EntityID   `json:"parent_id,omitempty"`
	Relation   Relation   `json:"relation,omitempty"`
	Placement  string     `json:"placement,omitempty"`
	Key        string     `json:"key,omitempty"`
	Value      string     `json:"value,omitempty"`
	Visibility Visibility `json:"visibility,omitempty"` // "public" or "private"
	Accessors  []EntityID `json:"accessors,omitempty"`  // Permitted observers when private
	Closed     *bool      `json:"closed,omitempty"`
	Locked     *bool      `json:"locked,omitempty"`
	Open       *bool      `json:"open,omitempty"`
}

// ActorIntent packages an actor's declared intent for scene adjudication.
type ActorIntent struct {
	ActorID     EntityID          `json:"actor_id"`
	ActorName   string            `json:"actor_name"`
	ActionType  actor.ActionType  `json:"action_type"`
	ActionText  string            `json:"action_text"`
	Targets     []EntityID        `json:"targets,omitempty"`
	Spontaneity float64           `json:"spontaneity"`
	Luck        float64           `json:"luck"`
	Attributes  map[string]string `json:"attributes,omitempty"`
}

// SceneAdjudication packages all concurrent actor intents and scene context for the Arbiter.
type SceneAdjudication struct {
	SceneID         SceneID
	Tick            int
	Timestamp       string
	Intents         []ActorIntent
	WorldState      string   // Objective, unfiltered description of the scene's spaces and entities
	SceneEvents     []string // Chronological recent occurrences in this scene
	GlobalEvents    []string // Digest of recent events across the wider simulation
	TickDurationSec int
}

// IntentVerdict provides the referee's judgment on an individual actor's intent.
type IntentVerdict struct {
	ActorID         EntityID `json:"actor_id"`
	Verdict         string   `json:"verdict"` // as_intended | altered | preempted
	Outcome         string   `json:"outcome"` // success | partial | failure | mishap | interrupted
	RemainingIntent string   `json:"remaining_intent,omitempty"`
}

// SceneRuling is the Arbiter's authoritative resolution of all concurrent actions in a scene.
type SceneRuling struct {
	SceneID         SceneID         `json:"scene_id"`
	Verdicts        []IntentVerdict `json:"verdicts"`
	ExecutionOrder  []EntityID      `json:"execution_order"` // Order in which actions physically materialized
	Resolution      string          `json:"resolution"`      // Objective third-person canonical truth of the scene
	Effects         []Effect        `json:"effects,omitempty"`
	AppliedEffects  []Effect        `json:"-"`
	RejectedEffects []string        `json:"-"`
	Usage           llm.Usage       `json:"-"`
	Duration        time.Duration   `json:"-"`
}

// Arbiter defines the contract for refereeing all concurrent actor intents in a scene.
type Arbiter interface {
	ResolveScene(ctx context.Context, adj SceneAdjudication) (*SceneRuling, error)
}

// LLMArbiter implements Arbiter using chat completions.
type LLMArbiter struct {
	client llm.Client
	model  string
}

// NewLLMArbiter creates an Arbiter backed by an LLM client.
func NewLLMArbiter(client llm.Client, model string) *LLMArbiter {
	if model == "" {
		model = "gemini-3.5-flash-lite"
	}
	return &LLMArbiter{
		client: client,
		model:  model,
	}
}

// ArbiterSchema constructs the strict JSON schema for scene adjudication rulings.
func ArbiterSchema(actorIDs []EntityID, entityIDs []string) *llm.Schema {
	var actorIDStrings []string
	for _, id := range actorIDs {
		actorIDStrings = append(actorIDStrings, string(id))
	}

	actorIDSchema := llm.Str("Actor ID")
	if len(actorIDStrings) > 0 {
		actorIDSchema = llm.Enum("Actor ID", actorIDStrings...)
	}

	entityIDSchema := llm.Str("Entity ID from world state")
	if len(entityIDs) > 0 {
		entityIDSchema = llm.Enum("Entity ID from world state", entityIDs...)
	}

	var verdictProps []llm.Property
	for _, id := range actorIDs {
		verdictProps = append(verdictProps, llm.Prop(string(id), llm.Obj(fmt.Sprintf("Adjudication verdict for %s.", id),
			llm.Prop("verdict", llm.Enum("Action verdict: as_intended (no major disruption), altered (adapted, deflected, partially interrupted), preempted (situation changed before action started).",
				"as_intended", "altered", "preempted")),
			llm.Prop("outcome", llm.Enum("Outcome calibration: success (succeeded as planned), partial (bounded progress), failure (failed cleanly), mishap (blunder or slip), interrupted (cut short mid-stride).",
				"success", "partial", "failure", "mishap", "interrupted")),
			llm.Prop("remaining_intent", llm.Nullable(llm.Str("Any uncompleted portion of intent if interrupted or altered; null if fully executed."))),
		)))
	}
	if len(verdictProps) == 0 {
		verdictProps = append(verdictProps, llm.Prop("dummy", llm.Str("Empty scene placeholder.")))
	}

	effectItem := llm.Obj("An atomic mutation to apply to the simulation world state.",
		llm.Prop("type", llm.Enum("Mutation type: move, set_attribute, remove_attribute, set_portal, set_open, reveal.",
			"move", "set_attribute", "remove_attribute", "set_portal", "set_open", "reveal")),
		llm.Prop("entity_id", entityIDSchema),
		llm.Prop("parent_id", llm.Nullable(entityIDSchema)),
		llm.Prop("relation", llm.Nullable(llm.Enum("Spatial relation for move: in, on, under, behind, worn, held, seated; null for other mutations.",
			"in", "on", "under", "behind", "worn", "held", "seated"))),
		llm.Prop("placement", llm.Nullable(llm.Str("Optional free-text placement qualifier (e.g. 'on coffee table'); null if none."))),
		llm.Prop("key", llm.Nullable(llm.Str("Attribute name for set_attribute or remove_attribute; null for other mutations."))),
		llm.Prop("value", llm.Nullable(llm.Str("New attribute value for set_attribute; null for other mutations."))),
		llm.Prop("visibility", llm.Nullable(llm.Enum("Attribute visibility: public (visible to all in room) or private (visible only to accessors); null for other mutations.",
			"public", "private"))),
		llm.Prop("accessors", llm.Nullable(llm.Arr("Actor IDs permitted to observe this private attribute; empty array if private to Arbiter only; null for other mutations.", actorIDSchema))),
		llm.Prop("closed", llm.Nullable(llm.Bool("Closed state for set_portal; null for other mutations."))),
		llm.Prop("locked", llm.Nullable(llm.Bool("Locked state for set_portal; null for other mutations."))),
		llm.Prop("open", llm.Nullable(llm.Bool("Open state for set_open; null for other mutations."))),
	)

	return llm.Obj("Authoritative scene adjudication ruling.",
		llm.Prop("execution_order", llm.Arr("Sequential order of actor actions based on spontaneity, reflexes, and physical position.", actorIDSchema)),
		llm.Prop("verdicts", llm.Obj("Verdicts keyed by actor ID.", verdictProps...)),
		llm.Prop("resolution", llm.Str("Concise objective third-person canonical ground truth of everything that physically happened in the scene. Written in past tense without second-person bias.")),
		llm.Prop("effects", llm.Arr("Atomic mutations applied to the world state.", effectItem)),
	)
}

type arbiterWireVerdict struct {
	Verdict         string  `json:"verdict"`
	Outcome         string  `json:"outcome"`
	RemainingIntent *string `json:"remaining_intent"`
}

type arbiterWireEffect struct {
	Type       string   `json:"type"`
	EntityID   string   `json:"entity_id"`
	ParentID   *string  `json:"parent_id"`
	Relation   *string  `json:"relation"`
	Placement  *string  `json:"placement"`
	Key        *string  `json:"key"`
	Value      *string  `json:"value"`
	Visibility *string  `json:"visibility"`
	Accessors  []string `json:"accessors"`
	Closed     *bool    `json:"closed"`
	Locked     *bool    `json:"locked"`
	Open       *bool    `json:"open"`
}

type arbiterWireResponse struct {
	ExecutionOrder []string                      `json:"execution_order"`
	Verdicts       map[string]arbiterWireVerdict `json:"verdicts"`
	Resolution     string                        `json:"resolution"`
	Effects        []arbiterWireEffect           `json:"effects"`
}

// ResolveScene refereees all concurrent actor intents in a scene and returns the authoritative ruling.
func (a *LLMArbiter) ResolveScene(ctx context.Context, adj SceneAdjudication) (*SceneRuling, error) {
	start := time.Now()

	systemPrompt := fmt.Sprintf(`You are THE ARBITER: an impartial, physically realistic simulation referee and physics engine.
Actors in this scene submitted their actions SIMULTANEOUSLY. You determine what ACTUALLY HAPPENS according to the structured schema.

REFEREE RULES:
1. Simultaneity & Precedence:
   - Actors acted concurrently this tick (%ds window).
   - Use Spontaneity scores and Luck rolls as guidance for who acts first or wins contests.
   - If two actors collide (e.g. both reaching for the same item), the higher spontaneity/reflexes and physical position wins; the second actor's action is "altered" or "preempted".
   - If an actor was interrupted before completing their action, mark verdict as "altered" or "preempted" with outcome "interrupted", and store unfinished action in "remaining_intent".
2. Outcome Calibration:
   - "mishap" (physical blunder or fumble) is especially likely when Luck <= 0.20 or under stressful conditions.
3. Objective Resolution Only:
   - Describe purely the objective physical ground truth of the scene in past tense.
   - Do NOT write subjective second-person text (the Broadcaster handles subjective perceptions).
4. The Attribute ACL & Visibility Principle:
   - "public": Openly observable by anyone in the room (e.g. visibly trembling, a shattered window, flickering lights). Affects everyone's perception and thinking. (Accessors are cleared automatically).
   - "private" WITH the entity in "accessors" (e.g. ["<entity_id>"]): Use this when you want an internal conscious state (e.g. racing heart, private sarcasm, conscious panic) to DIRECTLY AFFECT THAT ENTITY'S THINKING and future dialogue/choices.
   - "private" with NO accessors (empty []): Use this when you do NOT want the entity to inherently know about the attribute, but that attribute DOES AFFECT THE ACTUAL ACTIONS that you (the Arbiter) ultimately decide or resolve! (e.g. latent muscle cramp, subconscious panic impairing motor precision, an unseen structural defect). The entity remains oblivious in their conscious thoughts, but you use this hidden attribute to realistically alter, fumble, or restrict their physical attempts!`,
		adj.TickDurationSec)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("SCENE: %s | TICK: %d | TIME: %s | DURATION: %ds\n\n",
		adj.SceneID, adj.Tick, adj.Timestamp, adj.TickDurationSec))

	sb.WriteString("CONCURRENT ACTOR INTENTS (Submitted simultaneously this tick):\n")
	for _, in := range adj.Intents {
		targetsStr := "none"
		if len(in.Targets) > 0 {
			var tStrs []string
			for _, t := range in.Targets {
				tStrs = append(tStrs, string(t))
			}
			targetsStr = strings.Join(tStrs, ", ")
		}

		attrsStr := ""
		if len(in.Attributes) > 0 {
			var aStrs []string
			for k, v := range in.Attributes {
				aStrs = append(aStrs, fmt.Sprintf("%s=%s", k, v))
			}
			attrsStr = fmt.Sprintf(" | attrs: [%s]", strings.Join(aStrs, ", "))
		}

		sb.WriteString(fmt.Sprintf("- Actor: %s (ID: %s) | Type: %s | Spontaneity: %.2f | Luck: %.2f | Targets: [%s]%s\n  Intent: %q\n",
			in.ActorName, in.ActorID, in.ActionType, in.Spontaneity, in.Luck, targetsStr, attrsStr, in.ActionText))
	}

	sb.WriteString("\nOBJECTIVE WORLD STATE (Ground Truth of Scene's Spaces):\n")
	sb.WriteString(adj.WorldState)

	if len(adj.SceneEvents) > 0 {
		sb.WriteString("\n\nRECENT EVENTS IN THIS SCENE:\n")
		for _, ev := range adj.SceneEvents {
			sb.WriteString(fmt.Sprintf("- %s\n", ev))
		}
	}

	if len(adj.GlobalEvents) > 0 {
		sb.WriteString("\nRECENT GLOBAL EVENTS (Across Simulation):\n")
		for _, ev := range adj.GlobalEvents {
			sb.WriteString(fmt.Sprintf("- %s\n", ev))
		}
	}

	sb.WriteString("\nAdjudicate the scene:")

	var actorIDs []EntityID
	for _, in := range adj.Intents {
		actorIDs = append(actorIDs, in.ActorID)
	}
	entityIDs := extractEntityIDs(adj.WorldState, adj.Intents)

	req := llm.ChatRequest{
		Model: a.model,
		Messages: []llm.Message{
			{Role: llm.System, Content: systemPrompt},
			{Role: llm.User, Content: sb.String()},
		},
		ResponseFormat:   llm.StrictSchema("scene_ruling", "Authoritative scene adjudication ruling.", ArbiterSchema(actorIDs, entityIDs)),
		ReasoningEffort:  llm.ReasoningMedium,
		IncludeReasoning: true,
	}

	resp, err := a.client.ChatCompletion(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("arbiter: chat completion failed for scene %q: %w", adj.SceneID, err)
	}

	var wire arbiterWireResponse
	if err := llm.DecodeJSON(resp.Content, resp.ReasoningContent, &wire); err != nil {
		return nil, fmt.Errorf("arbiter: failed to decode scene ruling JSON: %w (content: %q)", err, resp.Content)
	}

	var ruling SceneRuling
	ruling.SceneID = adj.SceneID
	ruling.Duration = time.Since(start)
	ruling.Usage = resp.Usage
	ruling.Resolution = wire.Resolution

	for _, idStr := range wire.ExecutionOrder {
		ruling.ExecutionOrder = append(ruling.ExecutionOrder, EntityID(idStr))
	}

	verdictMap := wire.Verdicts
	emitted := make(map[string]bool)

	addVerdict := func(actorIDStr string) {
		if emitted[actorIDStr] {
			return
		}
		v, ok := verdictMap[actorIDStr]
		if !ok {
			return
		}
		emitted[actorIDStr] = true
		remaining := ""
		if v.RemainingIntent != nil {
			remaining = *v.RemainingIntent
		}
		ruling.Verdicts = append(ruling.Verdicts, IntentVerdict{
			ActorID:         EntityID(actorIDStr),
			Verdict:         v.Verdict,
			Outcome:         v.Outcome,
			RemainingIntent: remaining,
		})
	}

	for _, actorID := range ruling.ExecutionOrder {
		addVerdict(string(actorID))
	}
	for _, in := range adj.Intents {
		addVerdict(string(in.ActorID))
	}

	for _, we := range wire.Effects {
		effType := EffectType(we.Type)
		if we.Type == "set_condition" {
			effType = EffectSetAttribute
		}
		eff := Effect{
			Type:     effType,
			EntityID: EntityID(we.EntityID),
			Closed:   we.Closed,
			Locked:   we.Locked,
			Open:     we.Open,
		}
		if we.ParentID != nil {
			eff.ParentID = EntityID(*we.ParentID)
		}
		if we.Relation != nil {
			eff.Relation = Relation(*we.Relation)
		}
		if we.Placement != nil {
			eff.Placement = *we.Placement
		}
		if we.Key != nil {
			eff.Key = *we.Key
		}
		if we.Value != nil {
			eff.Value = *we.Value
		}
		if we.Visibility != nil {
			eff.Visibility = Visibility(*we.Visibility)
		}
		for _, acc := range we.Accessors {
			eff.Accessors = append(eff.Accessors, EntityID(acc))
		}
		ruling.Effects = append(ruling.Effects, eff)
	}

	return &ruling, nil
}

func extractEntityIDs(worldState string, intents []ActorIntent) []string {
	seen := make(map[string]bool)
	var ids []string

	addID := func(id string) {
		id = strings.TrimSpace(id)
		if id != "" && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}

	for _, in := range intents {
		addID(string(in.ActorID))
		for _, t := range in.Targets {
			addID(string(t))
		}
	}

	lines := strings.Split(worldState, "\n")
	for _, line := range lines {
		idx := strings.Index(line, "[ID: ")
		if idx != -1 {
			end := strings.Index(line[idx+5:], "]")
			if end != -1 {
				addID(line[idx+5 : idx+5+end])
			}
		}
	}

	if len(ids) > 250 {
		return nil
	}
	return ids
}

// orderEffects applies effects in the arbiter's execution order. Effects are attributed
// to the actor whose entity they touch (or who carries it); unattributed effects keep
// their relative order after the attributed ones. The sort is stable.
func orderEffects(r *SceneRuling) {
	if len(r.ExecutionOrder) == 0 || len(r.Effects) < 2 {
		return
	}
	rank := make(map[EntityID]int, len(r.ExecutionOrder))
	for i, id := range r.ExecutionOrder {
		if _, seen := rank[id]; !seen {
			rank[id] = i
		}
	}
	position := func(e Effect) int {
		for _, id := range []EntityID{e.EntityID, e.ParentID} {
			if p, ok := rank[id]; ok {
				return p
			}
		}
		return len(rank)
	}
	sort.SliceStable(r.Effects, func(i, j int) bool {
		return position(r.Effects[i]) < position(r.Effects[j])
	})
}
