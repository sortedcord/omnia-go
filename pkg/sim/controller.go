package sim

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"omnia.adityagupta.dev/pkg/actor"
	"omnia.adityagupta.dev/pkg/llm"
	"omnia.adityagupta.dev/pkg/memory"
)

// ErrQuit signals that a human controller chose to quit the simulation.
var ErrQuit = errors.New("sim: human controller requested quit")

// Turn represents the sensory context provided to a controller to elicit an action.
type Turn struct {
	ActorID          EntityID
	ActorName        string
	Tick             int
	Timestamp        string
	Stimulus         string // The LOD-rendered surroundings and new sensory events
	Relevant         map[string]float64
	Thoughts         string // Active thought context for human player viewing
	Attributes       map[string]string
	TargetCandidates []string // Candidate target names for action schema
	SelfContext      string   // Tier 3: Self-dossier context
	DossiersContext  string   // Tier 3: Activated dossiers context
	MemoriesContext  string   // Tier 2: Surfaced memories context
}

// Controller defines the interface for what directs an actor's decisions.
// It can be an autonomous LLM (LLMController) or a human operator (HumanController).
type Controller interface {
	Decide(ctx context.Context, mind *actor.Actor, turn Turn) (*actor.Reaction, error)
}

// LLMController is the default controller delegating decisions to the actor's LLM brain.
type LLMController struct{}

// NewLLMController creates an LLMController.
func NewLLMController() *LLMController {
	return &LLMController{}
}

// Decide invokes actor.React() using the actor's LLM model and prompt hierarchy.
func (c *LLMController) Decide(ctx context.Context, mind *actor.Actor, turn Turn) (*actor.Reaction, error) {
	if mind == nil {
		return nil, errors.New("llm controller: actor mind is nil")
	}
	stimulus := actor.SensoryEvent{
		Tick:            turn.Tick,
		Timestamp:       turn.Timestamp,
		Description:     turn.Stimulus,
		Relevant:        turn.Relevant,
		Attributes:      turn.Attributes,
		Targets:         turn.TargetCandidates,
		SelfContext:     turn.SelfContext,
		DossiersContext: turn.DossiersContext,
		MemoriesContext: turn.MemoriesContext,
	}
	return mind.React(ctx, stimulus)
}

// HumanController coordinates with an external interactive frontend (e.g. terminal, web UI).
// It presents the perceived Turn to a human, reads freeform natural language, and uses an LLM
// to classify the intent into an actor.Action while preserving player intent.
type HumanController struct {
	client   llm.Client
	model    string
	turns    chan Turn
	replies  chan string
	quitOnce sync.Once
	quitChan chan struct{}
}

// NewHumanController creates a HumanController ready for interactive I/O.
func NewHumanController(client llm.Client, model string) *HumanController {
	if client == nil {
		client = llm.DefaultClient()
	}
	if model == "" {
		model = "gemini-3.5-flash-lite"
	}
	return &HumanController{
		client:   client,
		model:    model,
		turns:    make(chan Turn),
		replies:  make(chan string),
		quitChan: make(chan struct{}),
	}
}

// Turns returns a read-only channel where new turns needing player input are sent.
func (h *HumanController) Turns() <-chan Turn {
	return h.turns
}

// Reply submits the human player's raw natural language input for the pending turn.
func (h *HumanController) Reply(input string) {
	select {
	case h.replies <- input:
	case <-h.quitChan:
	}
}

// Quit terminates the human controller and unblocks any waiting Decide calls.
func (h *HumanController) Quit() {
	h.quitOnce.Do(func() {
		close(h.quitChan)
	})
}

// Decide presents the turn to the human, awaits their input, and translates it to an Action.
func (h *HumanController) Decide(ctx context.Context, mind *actor.Actor, turn Turn) (*actor.Reaction, error) {
	// Deliver turn to the frontend
	select {
	case h.turns <- turn:
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-h.quitChan:
		return nil, ErrQuit
	}

	// Await the human player's response
	var rawInput string
	select {
	case rawInput = <-h.replies:
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-h.quitChan:
		return nil, ErrQuit
	}

	start := time.Now()

	// Translate natural language to structured Action
	action, err := h.interpret(ctx, mind, turn, rawInput)
	if err != nil {
		return nil, err
	}

	// Persist the choice into the character's subjective memory bank
	// so taking over and handing back preserves narrative continuity!
	if mind != nil {
		mind.Memory.RecordTurn(actor.TurnMemory{
			Timestamp: turn.Timestamp,
			Stimulus:  turn.Stimulus,
			Cognition: fmt.Sprintf("(Human player command: %q)", rawInput),
			Action:    action,
		})
		if mind.Mind != nil {
			mind.Mind.RecordTurn(memory.Turn{
				Tick:       turn.Tick,
				Perception: turn.Stimulus,
				Cognition:  fmt.Sprintf("(Human player command: %q)", rawInput),
				Action: memory.ActionRecord{
					Type:    action.Type.String(),
					Action:  action.Action,
					Targets: action.Targets,
				},
			})
		}
	}

	return &actor.Reaction{
		Cognition: fmt.Sprintf("(Player: %q)", rawInput),
		Action:    action,
		Duration:  time.Since(start),
	}, nil
}

// interpret converts freeform natural language into an actor.Action.
func (h *HumanController) interpret(ctx context.Context, mind *actor.Actor, turn Turn, rawInput string) (actor.Action, error) {
	trimmed := strings.TrimSpace(rawInput)
	if trimmed == "" {
		return actor.Action{
			Type:   actor.Wait,
			Action: "Hold position and observe.",
		}, nil
	}

	actorName := "the character"
	if mind != nil && mind.Profile.Name != "" {
		actorName = mind.Profile.Name
	}

	systemPrompt := fmt.Sprintf(`You are an intent interpreter for an interactive simulation.
A human player is controlling the character "%s".
Convert the player's natural language input into their intended action according to the structured schema.

RULES:
1. No God-Moding: If the player claims success (e.g. 'I shoot the drone and destroy it'), convert it into an ATTEMPT (e.g. 'Aim at the drone and fire'). The simulation's Arbiter will decide the actual outcome.
2. Pronoun Resolution: Use the provided surroundings to resolve vague references (e.g. if player says 'grab it', and stimulus mentions a scanner, action is 'Reach to grab the scanner').
3. Phrasing: Format "action" as a concise first-person/imperative intent (e.g. 'Lunge toward the manual breaker box').`, actorName)

	userPrompt := fmt.Sprintf("CURRENT SURROUNDINGS & PERCEPTION:\n%s\n\nPLAYER'S RAW INPUT:\n%q\n\nConvert to action JSON:",
		turn.Stimulus, rawInput)

	req := llm.ChatRequest{
		Model: h.model,
		Messages: []llm.Message{
			{Role: llm.System, Content: systemPrompt},
			{Role: llm.User, Content: userPrompt},
		},
		ResponseFormat: actor.ActionResponseFormat(turn.TargetCandidates),
	}

	resp, err := h.client.ChatCompletion(ctx, req)
	if err != nil {
		// Fallback: default to physical act with the raw string
		return actor.Action{
			Type:   actor.Act,
			Action: trimmed,
		}, nil
	}

	var action actor.Action
	if err := llm.DecodeJSON(resp.Content, resp.ReasoningContent, &action); err != nil {
		return actor.Action{
			Type:   actor.Act,
			Action: trimmed,
		}, nil
	}

	return action, nil
}
