package actor

import "strings"

// CleanCognition strips gateway-injected JSON verification boilerplate from the model's reasoning stream
// before it is persisted into character memory or displayed to players.
func CleanCognition(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}

	lines := strings.Split(raw, "\n")
	var cleaned []string
	skipSection := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)

		// Check for constraint validation section header
		if strings.Contains(lower, "validate against constraints") ||
			strings.Contains(lower, "review against constraints") ||
			strings.Contains(lower, "check constraints") {
			skipSection = true
			continue
		}

		// A new numbered or bold section header ends the skip section
		if skipSection && (strings.HasPrefix(trimmed, "1.") || strings.HasPrefix(trimmed, "2.") || strings.HasPrefix(trimmed, "3.") ||
			strings.HasPrefix(trimmed, "4.") || strings.HasPrefix(trimmed, "5.") || strings.HasPrefix(trimmed, "**")) {
			if !strings.Contains(lower, "constraint") && !strings.Contains(lower, "json") {
				skipSection = false
			}
		}

		if skipSection {
			continue
		}

		// Skip individual gateway-injected schema instruction checklist lines
		if strings.Contains(lower, "valid, parseable json") ||
			strings.Contains(lower, "scalars, boolean, null") ||
			strings.Contains(lower, "do not include any extra text outside") ||
			strings.Contains(lower, "no control tokens") ||
			strings.Contains(lower, "final json string formulation") ||
			strings.Contains(lower, "only generate a valid, parseable json") {
			continue
		}

		cleaned = append(cleaned, line)
	}

	return strings.TrimSpace(strings.Join(cleaned, "\n"))
}
