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
Your internal thinking process must be 100%% first-person, in-character stream of consciousness, raw subconscious reactions, and immediate visceral instincts as %s.
ABSOLUTELY NO META-ANALYSIS: Never mention prompts, instructions, formatting, schemas, JSON, 'the user', or 'the character'.
Do not write out JSON formatting checks or schema notes in your thoughts. Your thoughts must remain entirely in the narrative world as the crisis unfolds.`, actorName),
	}
}

// DefaultSystemPrompts returns the baseline system prompt suite for an actor.
func DefaultSystemPrompts(actorName string) []SystemPrompt {
	return []SystemPrompt{
		DefaultSimulationGrounding(),
		DefaultCognitionRule(actorName),
	}
}
