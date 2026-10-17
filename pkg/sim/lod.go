package sim

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"omnia.adityagupta.dev/pkg/llm"
)

// LODLevel represents the Level of Detail for rendering an entity to an actor.
type LODLevel int

const (
	LOD0 LODLevel = 0 // Omitted: not mentioned or perceivable in the current view
	LOD1 LODLevel = 1 // Collapsed summary: folded into parent or brief collective summary
	LOD2 LODLevel = 2 // Named with link: name and placement/relation visible
	LOD3 LODLevel = 3 // Full detail: name, description, exact placement, visible contents
)

// Summarizer provides automated summarization of entity subtrees for LOD 1 rendering.
type Summarizer interface {
	SummarizeSubtree(ctx context.Context, parent *Entity, children []*Entity) (string, error)
}

// LLMSummarizer generates contextual descriptions using chat completions.
type LLMSummarizer struct {
	client llm.Client
	model  string
}

// NewLLMSummarizer creates an LLM-powered subtree summarizer.
func NewLLMSummarizer(client llm.Client, model string) *LLMSummarizer {
	if model == "" {
		model = "gemini-3.5-flash-lite"
	}
	return &LLMSummarizer{
		client: client,
		model:  model,
	}
}

// SummarizeSubtree produces a short, evocative 3-8 word phrase summarizing an entity and its visible contents.
func (s *LLMSummarizer) SummarizeSubtree(ctx context.Context, parent *Entity, children []*Entity) (string, error) {
	if len(children) == 0 {
		return parent.Name, nil
	}

	var childNames []string
	for _, c := range children {
		if !c.Link.Concealed {
			childNames = append(childNames, c.Name)
		}
	}

	prompt := fmt.Sprintf("Describe '%s' containing [%s] in one brief, natural narrative phrase (maximum 8 words). Respond with only the phrase.",
		parent.Name, strings.Join(childNames, ", "))

	req := llm.ChatRequest{
		Model: s.model,
		Messages: []llm.Message{
			{Role: llm.System, Content: "You are a concise scene summarizer. Output only the short descriptive phrase."},
			{Role: llm.User, Content: prompt},
		},
	}

	resp, err := s.client.ChatCompletion(ctx, req)
	if err != nil {
		return "", err
	}

	clean := strings.Trim(strings.TrimSpace(resp.Content), "\"")
	if clean == "" {
		return "", fmt.Errorf("lod: empty summary returned by LLM")
	}
	return clean, nil
}

// ComputeSalience evaluates how prominent an entity is in the consciousness of a perceiver.
func (s *Simulation) ComputeSalience(perceiverID EntityID, targetID EntityID) (float64, LODLevel) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.computeSalienceLocked(perceiverID, targetID)
}

// RenderedPerception packages the formatted sensory text together with structured entity visibility.
type RenderedPerception struct {
	Text        string
	Seen        map[EntityID]LODLevel
	Perceivable []EntityRef
}

// RenderPerception constructs the subjective sensory text of an actor's immediate surroundings
// and carried gear, filtered dynamically through Level of Detail (LOD).
func (s *Simulation) RenderPerception(ctx context.Context, perceiverID EntityID) (string, error) {
	view, err := s.renderPerceptionView(ctx, perceiverID)
	return view.Text, err
}

// renderPerceptionView builds both the human/LLM-readable perception text and the structured
// catalog of visible entities for the Thought Distiller.
func (s *Simulation) renderPerceptionView(ctx context.Context, perceiverID EntityID) (RenderedPerception, error) {
	// Pre-pass: Discover any entities requiring an LLM summary so we do not hold the lock during network calls
	s.mu.RLock()
	perceiver, ok := s.Entities[perceiverID]
	if !ok {
		s.mu.RUnlock()
		return RenderedPerception{}, fmt.Errorf("lod: perceiver %q not found", perceiverID)
	}

	space, ok := s.whereLocked(perceiverID)
	if !ok || space == nil {
		s.mu.RUnlock()
		return RenderedPerception{}, fmt.Errorf("lod: perceiver %q is not situated in any enterable space", perceiverID)
	}

	type summaryWork struct {
		parent   *Entity
		children []*Entity
	}
	var toSummarize []summaryWork

	for _, childID := range space.Children {
		if childID == perceiverID {
			continue
		}
		child, ok := s.Entities[childID]
		if !ok {
			continue
		}
		_, lod := s.computeSalienceLocked(perceiverID, childID)
		if lod == LOD1 && (child.Summary == "" || child.SummaryRev != child.Revision) {
			var subEntities []*Entity
			for _, subID := range child.Children {
				if se, ok := s.Entities[subID]; ok && !se.Link.Concealed {
					subEntities = append(subEntities, se)
				}
			}
			toSummarize = append(toSummarize, summaryWork{
				parent:   child,
				children: subEntities,
			})
		}
	}
	s.mu.RUnlock()

	// Execute LLM summarization without holding any simulation lock
	for _, work := range toSummarize {
		if s.Summarizer != nil {
			if summary, err := s.Summarizer.SummarizeSubtree(ctx, work.parent, work.children); err == nil && summary != "" {
				s.mu.Lock()
				work.parent.Summary = summary
				work.parent.SummaryRev = work.parent.Revision
				s.mu.Unlock()
			}
		}
	}

	// Final render pass with lock
	s.mu.Lock()
	defer s.mu.Unlock()

	seen := make(map[EntityID]LODLevel)
	var perceivable []EntityRef

	// The current room is always perceivable
	perceivable = append(perceivable, EntityRef{
		ID:       space.ID,
		Name:     space.Name,
		Relation: "current location",
	})
	seen[space.ID] = LOD3

	var sb strings.Builder

	// 1. Current Location Header & Environmental State
	sb.WriteString(fmt.Sprintf("CURRENT LOCATION: %s\n", space.Name))
	if space.Description != "" {
		sb.WriteString(fmt.Sprintf("%s\n", space.Description))
	}
	if space.Space != nil && len(space.Space.Conditions) > 0 {
		sb.WriteString("Environmental Conditions:\n")
		// Deterministic sorted condition keys
		condKeys := make([]string, 0, len(space.Space.Conditions))
		for k := range space.Space.Conditions {
			condKeys = append(condKeys, k)
		}
		sort.Strings(condKeys)
		for _, k := range condKeys {
			sb.WriteString(fmt.Sprintf("- %s: %s\n", k, space.Space.Conditions[k]))
		}
	}

	// 2. Personal Equipment (Held, Worn, and Nested Sub-Items)
	var carriedLines []string
	var renderCarriedTree func(entityID EntityID, depth int)
	renderCarriedTree = func(entityID EntityID, depth int) {
		e, ok := s.Entities[entityID]
		if !ok {
			return
		}
		_, lod := s.computeSalienceLocked(perceiverID, entityID)
		if lod < LOD2 {
			return
		}

		seen[entityID] = lod
		perceivable = append(perceivable, EntityRef{
			ID:       e.ID,
			Name:     e.Name,
			Relation: string(e.Link.Relation),
		})

		indent := strings.Repeat("  ", depth)
		relDesc := string(e.Link.Relation)
		if e.Link.Placement != "" {
			relDesc = fmt.Sprintf("%s, %s", e.Link.Relation, e.Link.Placement)
		}
		if e.Link.Concealed {
			relDesc += ", concealed"
		}

		line := fmt.Sprintf("%s- %s [%s]", indent, e.Name, relDesc)
		if e.Description != "" && lod == LOD3 {
			line += fmt.Sprintf(": %s", e.Description)
		}
		carriedLines = append(carriedLines, line)

		for _, subID := range e.Children {
			renderCarriedTree(subID, depth+1)
		}
	}

	for _, childID := range perceiver.Children {
		renderCarriedTree(childID, 0)
	}

	if len(carriedLines) > 0 {
		sb.WriteString("\nEQUIPMENT ON YOUR PERSON:\n")
		sb.WriteString(strings.Join(carriedLines, "\n"))
		sb.WriteString("\n")
	}

	// 3. Co-Present Entities in the Space (LOD Filtered)
	var highDetailLines []string
	var mediumDetailLines []string
	var collapsedLines []string

	for _, childID := range space.Children {
		if childID == perceiverID {
			continue // Don't list ourselves
		}
		child, ok := s.Entities[childID]
		if !ok {
			continue
		}

		_, lod := s.computeSalienceLocked(perceiverID, childID)
		seen[childID] = lod

		switch lod {
		case LOD3:
			perceivable = append(perceivable, EntityRef{
				ID:       child.ID,
				Name:     child.Name,
				Relation: string(child.Link.Relation),
			})

			desc := child.Description
			if child.Link.Placement != "" {
				desc = fmt.Sprintf("[%s] %s", child.Link.Placement, desc)
			}
			highDetailLines = append(highDetailLines, fmt.Sprintf("- %s: %s", child.Name, desc))

			// Check visible children of this LOD 3 entity
			for _, subID := range child.Children {
				sub, ok := s.Entities[subID]
				if !ok || sub.Link.Concealed {
					continue
				}
				_, subLod := s.computeSalienceLocked(perceiverID, subID)
				seen[subID] = subLod
				if subLod >= LOD2 {
					perceivable = append(perceivable, EntityRef{
						ID:       sub.ID,
						Name:     sub.Name,
						Relation: string(sub.Link.Relation),
					})
					highDetailLines = append(highDetailLines, fmt.Sprintf("    * contains %s [%s]", sub.Name, sub.Link.Relation))
				}
			}

		case LOD2:
			perceivable = append(perceivable, EntityRef{
				ID:       child.ID,
				Name:     child.Name,
				Relation: string(child.Link.Relation),
			})
			placement := ""
			if child.Link.Placement != "" {
				placement = fmt.Sprintf(" (%s)", child.Link.Placement)
			}
			mediumDetailLines = append(mediumDetailLines, fmt.Sprintf("- %s%s", child.Name, placement))

		case LOD1:
			perceivable = append(perceivable, EntityRef{
				ID:       child.ID,
				Name:     child.Name,
				Relation: "in background",
			})
			summary := child.Summary
			if summary == "" {
				summary = child.Name
			}
			collapsedLines = append(collapsedLines, summary)

		case LOD0:
			// Omitted
		}
	}

	if len(highDetailLines) > 0 || len(mediumDetailLines) > 0 || len(collapsedLines) > 0 {
		sb.WriteString("\nVISIBLE IN THE IMMEDIATE SURROUNDINGS:\n")
		for _, line := range highDetailLines {
			sb.WriteString(line + "\n")
		}
		for _, line := range mediumDetailLines {
			sb.WriteString(line + "\n")
		}
		if len(collapsedLines) > 0 {
			sb.WriteString(fmt.Sprintf("In the background: %s\n", strings.Join(collapsedLines, ", ")))
		}
	}

	// 4. Visible Portals / Exits
	var portalLines []string
	for _, childID := range space.Children {
		child, ok := s.Entities[childID]
		if !ok || child.Portal == nil {
			continue
		}
		seen[childID] = LOD2
		perceivable = append(perceivable, EntityRef{
			ID:       child.ID,
			Name:     child.Name,
			Relation: "portal",
		})

		targetSpace, ok := s.Entities[child.Portal.Target]
		targetName := string(child.Portal.Target)
		if ok {
			targetName = targetSpace.Name
			perceivable = append(perceivable, EntityRef{
				ID:       targetSpace.ID,
				Name:     targetSpace.Name,
				Relation: "destination space",
			})
		}
		status := "open"
		if child.Portal.Closed {
			status = "closed"
			if child.Portal.Locked {
				status = "locked"
			}
		}
		dir := child.Portal.Direction
		if dir != "" {
			dir = fmt.Sprintf(" (%s)", dir)
		}
		portalLines = append(portalLines, fmt.Sprintf("- %s%s leads to %s [%s]", child.Name, dir, targetName, status))
	}
	if len(portalLines) > 0 {
		sb.WriteString("\nEXITS & PORTALS:\n")
		for _, line := range portalLines {
			sb.WriteString(line + "\n")
		}
	}

	return RenderedPerception{
		Text:        strings.TrimSpace(sb.String()),
		Seen:        seen,
		Perceivable: perceivable,
	}, nil
}

// computeSalienceLocked helper without mutex (caller must hold lock).
func (s *Simulation) computeSalienceLocked(perceiverID EntityID, targetID EntityID) (float64, LODLevel) {
	target, ok := s.Entities[targetID]
	if !ok {
		return 0, LOD0
	}

	isCarriedByPerceiver := s.isCarriedByLocked(targetID, perceiverID)

	// 1. Direct Concealment Check: Concealed items are invisible unless carried by perceiver themselves
	if target.Link.Concealed && !isCarriedByPerceiver {
		return 0, LOD0
	}

	// 2. Ancestor Visibility Check: If any ancestor is closed and opaque, contents are invisible to outsiders
	curr := target
	for curr.Parent != "" {
		parent, ok := s.Entities[curr.Parent]
		if !ok {
			break
		}
		if parent.ID == perceiverID {
			break // Inside perceiver's own equipment subtree
		}
		if parent.Link.Concealed && !isCarriedByPerceiver {
			return 0, LOD0
		}
		if parent.Space != nil && !parent.Space.Open && !parent.Space.Transparent {
			return 0, LOD0
		}
		curr = parent
	}

	// 3. Base Intrinsic Salience
	score := target.Salience

	// 4. Personal Attachment & Direct Interaction Boost
	if isCarriedByPerceiver {
		if target.Parent == perceiverID {
			if target.Link.Relation == RelHeld {
				score += 0.50
			} else if target.Link.Relation == RelWorn {
				score += 0.35
			}
		} else {
			// Sub-item in pocket or carried bag
			score += 0.25
		}
	}

	// 5. Dynamic Focus / Mention Boost (tracked per perceiver)
	if focusMap, hasPerceiver := s.FocusMap[perceiverID]; hasPerceiver {
		if boost, hasTarget := focusMap[targetID]; hasTarget {
			score += boost
		}
	}

	// 6. Container Nesting Penalty (further down the tree = less salient)
	nestingLevel := len(s.ancestorsLocked(targetID))
	if nestingLevel > 1 && !isCarriedByPerceiver {
		score -= float64(nestingLevel-1) * 0.12
	}

	// Clamp between 0.0 and 1.0
	if score > 1.0 {
		score = 1.0
	}
	if score < 0.0 {
		score = 0.0
	}

	var level LODLevel
	switch {
	case score >= 0.70:
		level = LOD3
	case score >= 0.38:
		level = LOD2
	case score >= 0.15:
		level = LOD1
	default:
		level = LOD0
	}

	return score, level
}

// isCarriedByLocked returns true if targetID is in actorID's equipment hierarchy.
func (s *Simulation) isCarriedByLocked(targetID, actorID EntityID) bool {
	curr, ok := s.Entities[targetID]
	if !ok {
		return false
	}
	for curr.Parent != "" {
		if curr.Parent == actorID {
			return true
		}
		parent, ok := s.Entities[curr.Parent]
		if !ok {
			break
		}
		curr = parent
	}
	return false
}

// ancestorsLocked returns all parent entities upwards without acquiring lock.
func (s *Simulation) ancestorsLocked(id EntityID) []*Entity {
	var result []*Entity
	curr, ok := s.Entities[id]
	if !ok {
		return result
	}
	for curr.Parent != "" {
		parent, ok := s.Entities[curr.Parent]
		if !ok {
			break
		}
		result = append(result, parent)
		curr = parent
	}
	return result
}

// BoostFocus raises the dynamic salience of an entity in a perceiver's focus.
func (s *Simulation) BoostFocus(perceiverID EntityID, targetID EntityID, amount float64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.FocusMap == nil {
		s.FocusMap = make(map[EntityID]map[EntityID]float64)
	}
	if s.FocusMap[perceiverID] == nil {
		s.FocusMap[perceiverID] = make(map[EntityID]float64)
	}
	s.FocusMap[perceiverID][targetID] += amount
}

// DecayFocus reduces all temporary focus boosts across ticks.
func (s *Simulation) DecayFocus(decayRate float64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for perceiverID, entityMap := range s.FocusMap {
		for targetID, boost := range entityMap {
			newBoost := boost * decayRate
			if newBoost < 0.05 {
				delete(entityMap, targetID)
			} else {
				entityMap[targetID] = newBoost
			}
		}
		if len(entityMap) == 0 {
			delete(s.FocusMap, perceiverID)
		}
	}
}

// ScanAndBoostMentions checks text against all entity names and aliases, boosting focus for matching entities.
func (s *Simulation) ScanAndBoostMentions(perceiverID EntityID, text string) {
	lower := strings.ToLower(text)

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.FocusMap == nil {
		s.FocusMap = make(map[EntityID]map[EntityID]float64)
	}
	if s.FocusMap[perceiverID] == nil {
		s.FocusMap[perceiverID] = make(map[EntityID]float64)
	}

	for id, entity := range s.Entities {
		if id == perceiverID {
			continue
		}
		matched := false
		if strings.Contains(lower, strings.ToLower(entity.Name)) {
			matched = true
		} else {
			for _, alias := range entity.Aliases {
				if strings.Contains(lower, strings.ToLower(alias)) {
					matched = true
					break
				}
			}
		}
		if matched {
			s.FocusMap[perceiverID][id] += 0.40
		}
	}
}
