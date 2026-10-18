package memory

import (
	"fmt"
	"strings"
)

// RenderPromptSections builds the developer context messages for Tier 2 and Tier 3 memory.
func RenderPromptSections(
	res RecallResult,
	subjects map[string]*Subject,
	selfBudgetTokens int,
	dossierBudgetTokens int,
	memoryBudgetTokens int,
) (selfText, dossiersText, memoriesText string) {
	if selfBudgetTokens <= 0 {
		selfBudgetTokens = 300
	}
	if dossierBudgetTokens <= 0 {
		dossierBudgetTokens = 700
	}
	if memoryBudgetTokens <= 0 {
		memoryBudgetTokens = 900
	}

	// 1. Section 4: Self-Dossier
	if res.SelfDossier != nil && (len(res.SelfDossier.Claims) > 0 || res.SelfDossier.Impression != "") {
		var sb strings.Builder
		sb.WriteString("YOUR CURRENT STATE OF MIND:\n")
		if res.SelfDossier.Impression != "" {
			sb.WriteString(fmt.Sprintf("Self-Impression: %s\n", res.SelfDossier.Impression))
		}

		var intentions, feelings, beliefs, suspicions []string
		for _, c := range res.SelfDossier.Claims {
			switch c.Kind {
			case ClaimIntention:
				intentions = append(intentions, "  * "+c.Content)
			case ClaimFeeling:
				feelings = append(feelings, "  * "+c.Content)
			case ClaimBelief:
				beliefs = append(beliefs, "  * "+c.Content)
			case ClaimSuspicion:
				suspicions = append(suspicions, "  * "+c.Content)
			}
		}

		if len(intentions) > 0 {
			sb.WriteString("Active Intentions & Goals:\n")
			sb.WriteString(strings.Join(intentions, "\n"))
			sb.WriteString("\n")
		}
		if len(feelings) > 0 {
			sb.WriteString("Current Feelings:\n")
			sb.WriteString(strings.Join(feelings, "\n"))
			sb.WriteString("\n")
		}
		if len(beliefs) > 0 {
			sb.WriteString("Current Convictions:\n")
			sb.WriteString(strings.Join(beliefs, "\n"))
			sb.WriteString("\n")
		}
		if len(suspicions) > 0 {
			sb.WriteString("Doubts & Suspicions:\n")
			sb.WriteString(strings.Join(suspicions, "\n"))
			sb.WriteString("\n")
		}

		selfText = trimToTokenBudget(sb.String(), selfBudgetTokens)
	}

	// 2. Section 5: Activated Dossiers ("What you think of...")
	if len(res.ActivatedDossiers) > 0 {
		var sb strings.Builder
		sb.WriteString("WHAT YOU THINK OF PEOPLE & PLACES IN MIND:\n")
		for i, d := range res.ActivatedDossiers {
			if i > 0 {
				sb.WriteString("\n")
			}
			displayName := d.SubjectID
			if subj, ok := subjects[d.SubjectID]; ok && len(subj.Names) > 0 {
				displayName = fmt.Sprintf("%s (%s)", subj.Names[0], d.SubjectID)
			}

			sb.WriteString(fmt.Sprintf("- Subject: %s\n", displayName))
			if d.Impression != "" {
				sb.WriteString(fmt.Sprintf("  Impression: %s\n", d.Impression))
			}

			// Show active claims
			for _, c := range d.Claims {
				sb.WriteString(fmt.Sprintf("  * [%s] %s\n", c.Kind, c.Content))
			}
		}
		dossiersText = trimToTokenBudget(sb.String(), dossierBudgetTokens)
	}

	// 3. Section 6: Surfaced Memories
	if len(res.SurfacedEpisodes) > 0 {
		var sb strings.Builder
		sb.WriteString("MEMORIES SURFACING IN YOUR MIND:\n")
		for i, ep := range res.SurfacedEpisodes {
			if i > 0 {
				sb.WriteString("\n")
			}
			if res.VividEpisodes[ep.ID] {
				sb.WriteString(ep.FormatVivid())
			} else {
				sb.WriteString(ep.FormatSummary())
			}
			sb.WriteString("\n")
		}
		memoriesText = trimToTokenBudget(sb.String(), memoryBudgetTokens)
	}

	return selfText, dossiersText, memoriesText
}

func trimToTokenBudget(text string, maxTokens int) string {
	maxChars := maxTokens * 4
	if len(text) <= maxChars {
		return strings.TrimSpace(text)
	}
	// Trim at last line before limit
	idx := strings.LastIndex(text[:maxChars], "\n")
	if idx != -1 {
		return strings.TrimSpace(text[:idx])
	}
	return strings.TrimSpace(text[:maxChars])
}
