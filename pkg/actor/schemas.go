package actor

import "omnia.adityagupta.dev/pkg/llm"

// ActionSchema returns the strict JSON schema for character manifest actions.
// If targetNames is non-empty, targets array items are constrained to those exact names.
func ActionSchema(targetNames []string) *llm.Schema {
	targetsItemSchema := llm.Str("Name of entity or character addressed or targeted.")
	if len(targetNames) > 0 {
		targetsItemSchema = llm.Enum("Name of entity or character addressed or targeted.", targetNames...)
	}

	return llm.Obj("A character manifest turn declaring verbal speech, physical action, or deliberate waiting.",
		llm.Prop("action_type", llm.Enum("Category of engagement: speak (dialogue), act (physical attempt), wait (holding position).", "speak", "act", "wait")),
		llm.Prop("action", llm.Str("In-character dialogue spoken aloud, description of physical attempt, or reason for waiting.")),
		llm.Prop("targets", llm.Arr("Intended addressees or interactive targets. Empty if addressing the room or self.", targetsItemSchema)),
	)
}

// ActionResponseFormat constructs a strict Structured Outputs format for actor actions.
func ActionResponseFormat(targetNames []string) *llm.ResponseFormat {
	return llm.StrictSchema("actor_action", "A character manifest turn declaring verbal speech, physical actions, or deliberate waiting.", ActionSchema(targetNames))
}
