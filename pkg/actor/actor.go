package actor

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"omnia.adityagupta.dev/pkg/llm"
)

// Option allows configuring an Actor during creation.
type Option func(*Actor)

// WithModel sets the model to use for this actor.
func WithModel(model string) Option {
	return func(a *Actor) {
		a.model = model
	}
}

// WithReasoningEffort sets the reasoning deliberation effort.
func WithReasoningEffort(effort llm.ReasoningEffort) Option {
	return func(a *Actor) {
		a.reasoningEffort = effort
	}
}

// WithClient allows injecting a custom or mock LLM client.
func WithClient(client llm.Client) Option {
	return func(a *Actor) {
		a.client = client
	}
}

// WithAPI configures a direct HTTP backend endpoint and API key.
func WithAPI(baseURL, apiKey string) Option {
	return func(a *Actor) {
		a.client = llm.NewHTTPClient(baseURL, apiKey)
	}
}

// WithStatus sets the initial physical/mental status of the actor.
func WithStatus(status Status) Option {
	return func(a *Actor) {
		a.Status = status
	}
}

// WithMemory sets the initial subjective memory bank of the actor.
func WithMemory(memory *Memory) Option {
	return func(a *Actor) {
		a.Memory = memory
	}
}

// WithSystemPrompts completely replaces the actor's system prompt list.
func WithSystemPrompts(prompts ...SystemPrompt) Option {
	return func(a *Actor) {
		a.SystemPrompts = prompts
	}
}

// WithSystemPrompt adds or overrides a single named system prompt.
func WithSystemPrompt(prompt SystemPrompt) Option {
	return func(a *Actor) {
		a.SetSystemPrompt(prompt)
	}
}

// Actor represents an autonomous, subjective entity in the simulation.
type Actor struct {
	Profile       Profile
	Status        Status
	Memory        *Memory
	SystemPrompts []SystemPrompt

	client          llm.Client
	model           string
	reasoningEffort llm.ReasoningEffort
}

// New creates a new Actor with the provided profile and optional configurations.
// If no client is specified, it automatically connects via llm.DefaultClient().
func New(profile Profile, opts ...Option) *Actor {
	a := &Actor{
		Profile:         profile,
		Memory:          NewMemory(),
		SystemPrompts:   DefaultSystemPrompts(profile.Name),
		model:           "gemini-3.5-flash-lite",
		reasoningEffort: llm.ReasoningHigh,
	}

	for _, opt := range opts {
		opt(a)
	}

	if a.client == nil {
		a.client = llm.DefaultClient()
	}

	return a
}

// SetSystemPrompt adds or replaces a named system prompt.
func (a *Actor) SetSystemPrompt(prompt SystemPrompt) {
	for i, sp := range a.SystemPrompts {
		if sp.Name == prompt.Name {
			a.SystemPrompts[i] = prompt
			return
		}
	}
	a.SystemPrompts = append(a.SystemPrompts, prompt)
}

// RemoveSystemPrompt removes a system prompt by its name.
func (a *Actor) RemoveSystemPrompt(name string) {
	filtered := make([]SystemPrompt, 0, len(a.SystemPrompts))
	for _, sp := range a.SystemPrompts {
		if sp.Name != name {
			filtered = append(filtered, sp)
		}
	}
	a.SystemPrompts = filtered
}

// AddThought seeds or inserts a thought directly into the actor's memory.
func (a *Actor) AddThought(t Thought) Thought {
	return a.Memory.AddThought(t)
}

// ThoughtsSnapshot returns a thread-safe copy of the actor's current thoughts.
func (a *Actor) ThoughtsSnapshot() []Thought {
	return a.Memory.ThoughtsSnapshot()
}

// ApplyThoughtOps applies mutations from the Thought Distiller to memory.
func (a *Actor) ApplyThoughtOps(tick int, ops []ThoughtOp, known map[string]string) ([]ThoughtOp, []string) {
	return a.Memory.ApplyThoughtOps(tick, ops, known)
}

// LearnEntities records perceived entities into the actor's subjective knowledge set.
func (a *Actor) LearnEntities(entities map[string]string) {
	a.Memory.LearnEntities(entities)
}

// KnownEntitiesCopy returns all entity IDs and names known to this actor.
func (a *Actor) KnownEntitiesCopy() map[string]string {
	return a.Memory.KnownEntitiesCopy()
}

// SelectThoughts picks top-K thoughts for prompt injection or inspection.
func (a *Actor) SelectThoughts(tick int, relevant map[string]float64, maxCount int) []Thought {
	return a.Memory.SelectThoughts(tick, relevant, maxCount)
}

// FormatThoughtContext formats the actor's active state of mind.
func (a *Actor) FormatThoughtContext(tick int, relevant map[string]float64) string {
	return a.Memory.FormatThoughtContext(tick, relevant)
}

// React processes a subjective sensory stimulus, deliberates internally via reasoning tokens,
// emits a manifest action into the world, and updates subjective memory.
func (a *Actor) React(ctx context.Context, stimulus SensoryEvent) (*Reaction, error) {
	messages := a.buildMessages(stimulus)

	req := llm.ChatRequest{
		Model:            a.model,
		Messages:         messages,
		ResponseFormat:   "json_object",
		ReasoningEffort:  a.reasoningEffort,
		IncludeReasoning: true,
	}

	resp, err := a.client.ChatCompletion(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("actor %q failed to react: %w", a.Profile.Name, err)
	}

	var action Action
	if err := llm.DecodeJSON(resp.Content, resp.ReasoningContent, &action); err != nil {
		return nil, fmt.Errorf("actor %q returned malformed action JSON: %w (content: %q)", a.Profile.Name, err, resp.Content)
	}

	// Persist this subjective experience into the actor's memory bank
	a.Memory.RecordTurn(TurnMemory{
		Timestamp: stimulus.Timestamp,
		Stimulus:  stimulus.Description,
		Cognition: resp.ReasoningContent,
		Action:    action,
	})

	return &Reaction{
		Cognition: resp.ReasoningContent,
		Action:    action,
		Usage:     resp.Usage,
		Duration:  resp.Duration,
	}, nil
}

func (a *Actor) buildMessages(currentStimulus SensoryEvent) []llm.Message {
	messages := make([]llm.Message, 0)

	// 1. Modular System Messages: Invariant simulation rules, cognition rules, and schema contract
	messages = append(messages, a.buildSystemMessages()...)

	// 2. Modular Developer Messages: Identity, status, and subjective memory state
	messages = append(messages, a.buildDeveloperMessages()...)

	// 3. Subjective Turn History
	turns := a.Memory.Turns
	if a.Memory.MaxHistoryTurns > 0 && len(turns) > a.Memory.MaxHistoryTurns {
		turns = turns[len(turns)-a.Memory.MaxHistoryTurns:]
	}

	for _, turn := range turns {
		// Sensory ingress experienced by this actor
		stimulusText := turn.Stimulus
		if turn.Timestamp != "" {
			stimulusText = fmt.Sprintf("[SIMULATION SENSORY EVENT: %s]\n%s", turn.Timestamp, turn.Stimulus)
		}
		messages = append(messages, llm.Message{
			Role:    llm.User,
			Content: stimulusText,
		})

		// Manifest action previously emitted by this actor
		actionJSON, _ := json.Marshal(turn.Action)
		messages = append(messages, llm.Message{
			Role:    llm.Assistant,
			Content: string(actionJSON),
		})
	}

	// 4. Current State of Mind (Grouped Developer Thoughts Message)
	thoughtContext := a.Memory.FormatThoughtContext(currentStimulus.Tick, currentStimulus.Relevant)
	if strings.TrimSpace(thoughtContext) != "" {
		messages = append(messages, llm.Message{
			Role:    llm.Developer,
			Content: strings.TrimSpace(thoughtContext),
		})
	}

	// 5. Current Sensory Stimulus
	messages = append(messages, llm.Message{
		Role:    llm.User,
		Content: currentStimulus.String(),
	})

	return messages
}

// buildSystemMessages maps the actor's configured named system prompts to llm.Message items.
func (a *Actor) buildSystemMessages() []llm.Message {
	msgs := make([]llm.Message, 0, len(a.SystemPrompts))
	for _, sp := range a.SystemPrompts {
		if strings.TrimSpace(sp.Content) == "" {
			continue
		}
		msgs = append(msgs, llm.Message{
			Role:    llm.System,
			Content: sp.Content,
		})
	}
	return msgs
}

// buildDeveloperMessages modularizes the actor's profile, dynamic state, and subjective memory.
func (a *Actor) buildDeveloperMessages() []llm.Message {
	msgs := make([]llm.Message, 0, 3)

	// 1. Profile & Core Identity
	var directives []string
	for _, d := range a.Profile.Directives {
		directives = append(directives, "  * "+d)
	}
	directivesText := strings.Join(directives, "\n")
	if directivesText != "" {
		directivesText = "\n- Directives:\n" + directivesText
	}

	profileContent := fmt.Sprintf(`ACTOR PROFILE:
- Name: %s
- Personality: %s%s
- Background: %s

COGNITION DIRECTIVE:
When thinking, write exclusively as %s's raw, unformatted stream of consciousness. Never use numbered lists, bullet points, headers, or analytical steps.`,
		a.Profile.Name, a.Profile.Personality, directivesText, a.Profile.Background, a.Profile.Name)

	msgs = append(msgs, llm.Message{
		Role:    llm.Developer,
		Content: profileContent,
	})

	// 2. Somatic & Internal Status
	var vitalsKeys []string
	for k := range a.Status.Vitals {
		vitalsKeys = append(vitalsKeys, k)
	}
	sort.Strings(vitalsKeys)
	var vitals []string
	for _, k := range vitalsKeys {
		vitals = append(vitals, fmt.Sprintf("  * %s: %s", k, a.Status.Vitals[k]))
	}
	vitalsText := strings.Join(vitals, "\n")
	if vitalsText != "" {
		vitalsText = "\n- Vitals:\n" + vitalsText
	}

	statusContent := fmt.Sprintf(`CURRENT INTERNAL STATUS:
- Emotional State: %s%s`,
		a.Status.Mood, vitalsText)

	msgs = append(msgs, llm.Message{
		Role:    llm.Developer,
		Content: statusContent,
	})

	return msgs
}
