package actor

import "fmt"

// SystemPrompt represents a named, modular system directive.
type SystemPrompt struct {
	Name    string
	Content string
}

// Canonical standard system prompt names
const (
	PromptSimulationGrounding = "simulation_grounding"
	PromptCognitionRule       = "cognition_rule"
)

// DefaultSimulationGrounding establishes the actor's existence inside the sandbox.
func DefaultSimulationGrounding() SystemPrompt {
	return SystemPrompt{
		Name: PromptSimulationGrounding,
		Content: `You are an autonomous sentient actor within an ongoing high-fidelity sandbox simulation.
Stay strictly in character at all times. Never break character or acknowledge being an AI.`,
	}
}

// DefaultCognitionRule forces the reasoning stream into pure first-person in-character thoughts.
func DefaultCognitionRule(actorName string) SystemPrompt {
	return SystemPrompt{
		Name: PromptCognitionRule,
		Content: fmt.Sprintf(`CRITICAL COGNITION RULE:
Your reasoning stream is your private, in-character stream of consciousness as %s.
The output format is handled automatically by the system; do NOT analyze, checklist, or mention JSON, formatting, schemas, or instructions in your thoughts.
Immerse yourself completely in %s's authentic subjective reality: immediate visceral reactions, emotional state, physical surroundings, and tactical instincts.`, actorName, actorName),
	}
}

// DefaultSystemPrompts returns the baseline system prompt suite for an actor.
func DefaultSystemPrompts(actorName string) []SystemPrompt {
	return []SystemPrompt{
		DefaultSimulationGrounding(),
		DefaultCognitionRule(actorName),
	}
}
