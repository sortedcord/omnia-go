package scenario

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"omnia.adityagupta.dev/pkg/scenario/spec"
)

var slugRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// Severity defines the impact level of a validation issue.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityInfo    Severity = "info"
)

// Issue represents a specific semantic validation failure, warning, or lint note.
type Issue struct {
	Pointer  string   `json:"pointer"`
	RecordID string   `json:"record_id"`
	Severity Severity `json:"severity"`
	Code     string   `json:"code"`
	Message  string   `json:"message"`
}

// ValidationReport contains all issues discovered during semantic inspection.
type ValidationReport struct {
	Valid                bool     `json:"valid"`
	Issues               []Issue  `json:"issues"`
	ActorsReachedOnTick1 []string `json:"actors_reached_on_tick_1,omitempty"`
}

var (
	validEntityKinds    = []string{"space", "item", "portal", "actor"}
	validRelations      = []string{"in", "on", "under", "behind", "worn", "held", "seated", "contained"}
	validClaimKinds     = []string{"belief", "feeling", "suspicion", "intention"}
	validSubjectKinds   = []string{"person", "place", "object", "group", "idea"}
	validSubjectOrigins = []string{"authored", "perceived"}
	validBindingModes   = []string{"identity", "anchor"}
	validPrecisions     = []string{"minute", "hour", "day", "month", "year", ""}
	validVisibilities   = []string{"public", "private", ""}
)

// Validate inspects a decoded scenario Document against all semantic and architectural constraints.
func Validate(doc *spec.Document, startID string) ValidationReport {
	var issues []Issue
	add := func(pointer, recordID string, sev Severity, code, msg string) {
		issues = append(issues, Issue{
			Pointer:  pointer,
			RecordID: recordID,
			Severity: sev,
			Code:     code,
			Message:  msg,
		})
	}

	if doc == nil {
		add("/", "", SeverityError, "NIL_DOC", "document cannot be nil")
		return ValidationReport{Valid: false, Issues: issues}
	}

	// 1. Meta & Identity Validation
	if doc.Meta.ID == "" {
		add("/meta/id", "", SeverityError, "MISSING_ID", "meta.id is required")
	} else if !slugRegex.MatchString(doc.Meta.ID) {
		add("/meta/id", doc.Meta.ID, SeverityError, "INVALID_SLUG", "meta.id must be a slug: ^[a-z0-9][a-z0-9_-]{0,63}$")
	}

	checkSlug := func(id, pointer, kind string) bool {
		if id == "" {
			add(pointer, "", SeverityError, "MISSING_ID", fmt.Sprintf("%s id cannot be empty", kind))
			return false
		}
		if !slugRegex.MatchString(id) {
			add(pointer, id, SeverityError, "INVALID_SLUG", fmt.Sprintf("%s id %q must be a slug", kind, id))
			return false
		}
		return true
	}

	charMap := make(map[string]spec.Character)
	charIDs := make(map[string]string)
	for i, c := range doc.Characters {
		ptr := fmt.Sprintf("/characters/%d", i)
		if checkSlug(c.ID, ptr+"/id", "character") {
			if prev, exists := charIDs[c.ID]; exists {
				add(ptr+"/id", c.ID, SeverityError, "DUPLICATE_ID", fmt.Sprintf("duplicate character ID %q (previously declared at %s)", c.ID, prev))
			} else {
				charIDs[c.ID] = ptr
			}
		}
		charMap[c.ID] = c
	}

	entityMap := make(map[string]spec.Entity)
	entityIDs := make(map[string]string)
	actorEntities := make(map[string]bool)
	spaceEntities := make(map[string]bool)
	portalEntities := make(map[string]bool)

	for i, e := range doc.Entities {
		ptr := fmt.Sprintf("/entities/%d", i)
		if checkSlug(e.ID, ptr+"/id", "entity") {
			if prev, exists := entityIDs[e.ID]; exists {
				add(ptr+"/id", e.ID, SeverityError, "DUPLICATE_ID", fmt.Sprintf("duplicate entity ID %q (previously declared at %s)", e.ID, prev))
			} else {
				entityIDs[e.ID] = ptr
			}
		}
		entityMap[e.ID] = e

		if !slices.Contains(validEntityKinds, e.Kind) {
			add(ptr+"/kind", e.ID, SeverityError, "INVALID_ENUM", fmt.Sprintf("invalid entity kind %q; allowed: %v", e.Kind, validEntityKinds))
		}
		if e.Link != nil && !slices.Contains(validRelations, e.Link.Relation) {
			add(ptr+"/link/relation", e.ID, SeverityError, "INVALID_ENUM", fmt.Sprintf("invalid relation %q; allowed: %v", e.Link.Relation, validRelations))
		}

		if e.Salience < 0 || e.Salience > 1.0 {
			add(ptr+"/salience", e.ID, SeverityError, "OUT_OF_RANGE", "entity salience must be within [0.0, 1.0]")
		}

		switch e.Kind {
		case "actor":
			actorEntities[e.ID] = true
			if e.Actor == nil {
				add(ptr+"/actor", e.ID, SeverityError, "MISSING_BLOCK", "actor kind requires 'actor' block")
			} else {
				if _, ok := charMap[e.Actor.CharacterID]; !ok {
					add(ptr+"/actor/character_id", e.ID, SeverityError, "UNKNOWN_REF", fmt.Sprintf("actor references unknown character %q", e.Actor.CharacterID))
				}
			}
		case "space":
			spaceEntities[e.ID] = true
			if e.Space == nil {
				add(ptr+"/space", e.ID, SeverityError, "MISSING_BLOCK", "space kind requires 'space' block")
			}
		case "portal":
			portalEntities[e.ID] = true
			if e.Portal == nil {
				add(ptr+"/portal", e.ID, SeverityError, "MISSING_BLOCK", "portal kind requires 'portal' block")
			}
		}

		// Attribute validation
		seenKeys := make(map[string]bool)
		for j, attr := range e.Attributes {
			aPtr := fmt.Sprintf("%s/attributes/%d", ptr, j)
			if seenKeys[attr.Key] {
				add(aPtr+"/key", e.ID, SeverityError, "DUPLICATE_KEY", fmt.Sprintf("duplicate attribute key %q on entity", attr.Key))
			}
			seenKeys[attr.Key] = true
			if attr.Visibility != "" && !slices.Contains(validVisibilities, attr.Visibility) {
				add(aPtr+"/visibility", e.ID, SeverityError, "INVALID_ENUM", fmt.Sprintf("invalid visibility %q", attr.Visibility))
			}
			if len(attr.Allowed) > 0 && !slices.Contains(attr.Allowed, attr.Value) {
				add(aPtr+"/value", e.ID, SeverityError, "INVALID_VALUE", fmt.Sprintf("attribute value %q not in allowed enum %v", attr.Value, attr.Allowed))
			}
			if attr.Key == "arousal" && !slices.Contains([]string{"low", "normal", "high"}, attr.Value) {
				add(aPtr+"/value", e.ID, SeverityError, "INVALID_CORE_VALUE", "core attribute 'arousal' must be 'low', 'normal', or 'high'")
			}
		}
	}

	// 2. Spatial Hierarchy & Portals
	for i, e := range doc.Entities {
		ptr := fmt.Sprintf("/entities/%d", i)
		if e.ParentID != "" {
			parent, ok := entityMap[e.ParentID]
			if !ok {
				add(ptr+"/parent_id", e.ID, SeverityError, "UNKNOWN_PARENT", fmt.Sprintf("parent entity %q not found", e.ParentID))
			} else {
				if e.Kind == "space" && e.Space != nil && e.Space.Enterable {
					if parent.Kind == "space" && parent.Space != nil && parent.Space.Enterable {
						add(ptr+"/parent_id", e.ID, SeverityError, "NESTED_ENTERABLE_SPACE",
							"enterable spaces cannot be children of other enterable spaces; join them with portals and use 'class' for building grouping")
					}
				}
			}
		} else {
			if e.Kind != "space" {
				add(ptr+"/parent_id", e.ID, SeverityError, "ROOT_MUST_BE_SPACE", fmt.Sprintf("non-space entity %q has no parent", e.ID))
			}
		}

		// Check portal endpoints
		if e.Kind == "portal" && e.Portal != nil {
			if !spaceEntities[e.Portal.FromID] {
				add(ptr+"/portal/from_id", e.ID, SeverityError, "UNKNOWN_SPACE", fmt.Sprintf("portal from_id %q is not an existing space", e.Portal.FromID))
			}
			if !spaceEntities[e.Portal.ToID] {
				add(ptr+"/portal/to_id", e.ID, SeverityError, "UNKNOWN_SPACE", fmt.Sprintf("portal to_id %q is not an existing space", e.Portal.ToID))
			}
		}
	}

	// Check tree acyclicity
	visitedTree := make(map[string]bool)
	for id := range entityMap {
		curr := id
		path := make(map[string]bool)
		for curr != "" {
			if path[curr] {
				add("/entities", id, SeverityError, "CYCLIC_PARENT", fmt.Sprintf("cycle detected in entity parent hierarchy involving %q", curr))
				break
			}
			path[curr] = true
			if visitedTree[curr] {
				break
			}
			curr = entityMap[curr].ParentID
		}
		visitedTree[id] = true
	}

	// 3. History Validation
	historyIDs := make(map[string]string)
	for i, h := range doc.History {
		ptr := fmt.Sprintf("/history/%d", i)
		if checkSlug(h.ID, ptr+"/id", "history") {
			if prev, exists := historyIDs[h.ID]; exists {
				add(ptr+"/id", h.ID, SeverityError, "DUPLICATE_ID", fmt.Sprintf("duplicate history ID %q (previously declared at %s)", h.ID, prev))
			} else {
				historyIDs[h.ID] = ptr
			}
		}
		if h.LocationID != "" && !spaceEntities[h.LocationID] {
			add(ptr+"/location_id", h.ID, SeverityError, "UNKNOWN_SPACE", fmt.Sprintf("history event location %q not found", h.LocationID))
		}
		for _, pid := range h.ParticipantIDs {
			if _, ok := charMap[pid]; !ok {
				add(ptr+"/participant_ids", h.ID, SeverityError, "UNKNOWN_CHAR", fmt.Sprintf("history participant %q not found", pid))
			}
		}
	}

	// 4. Mind Structure Validation (Memory Invariant I1 & I2)
	for i, m := range doc.Minds {
		mPtr := fmt.Sprintf("/minds/%d", i)
		if _, ok := charMap[m.OwnerID]; !ok {
			add(mPtr+"/owner_id", m.OwnerID, SeverityError, "UNKNOWN_OWNER", fmt.Sprintf("mind owner %q does not exist in characters", m.OwnerID))
		}

		mindSubjectIDs := make(map[string]bool)
		mindSubjectMap := make(map[string]spec.Subject)
		for j, s := range m.Subjects {
			sPtr := fmt.Sprintf("%s/subjects/%d", mPtr, j)
			if s.ID == "" {
				add(sPtr+"/id", "", SeverityError, "MISSING_ID", "subject id cannot be empty")
			} else if !slugRegex.MatchString(s.ID) {
				add(sPtr+"/id", s.ID, SeverityError, "INVALID_SLUG", fmt.Sprintf("subject id %q must be a slug", s.ID))
			}
			if !slices.Contains(validSubjectKinds, s.Kind) {
				add(sPtr+"/kind", s.ID, SeverityError, "INVALID_ENUM", fmt.Sprintf("invalid subject kind %q; allowed: %v", s.Kind, validSubjectKinds))
			}
			if !slices.Contains(validSubjectOrigins, s.Origin) {
				add(sPtr+"/origin", s.ID, SeverityError, "INVALID_ENUM", fmt.Sprintf("invalid subject origin %q; allowed: %v", s.Origin, validSubjectOrigins))
			}
			if mindSubjectIDs[s.ID] {
				add(sPtr+"/id", s.ID, SeverityError, "DUPLICATE_ID", fmt.Sprintf("duplicate subject ID %q in mind of %s", s.ID, m.OwnerID))
			}
			mindSubjectIDs[s.ID] = true
			mindSubjectMap[s.ID] = s

			// Binding checks (syntax only - I2)
			for k, b := range s.Bindings {
				bPtr := fmt.Sprintf("%s/bindings/%d", sPtr, k)
				if !slices.Contains(validBindingModes, b.Mode) {
					add(bPtr+"/mode", s.ID, SeverityError, "INVALID_ENUM", fmt.Sprintf("invalid binding mode %q; allowed: %v", b.Mode, validBindingModes))
				}
				if b.Confidence < 0 || b.Confidence > 1.0 {
					add(bPtr+"/confidence", s.ID, SeverityError, "OUT_OF_RANGE", "binding confidence must be in [0.0, 1.0]")
				}
				if _, ok := entityMap[b.EntityID]; !ok {
					// Fallible reality desync: emit INFO, never ERROR
					add(bPtr+"/entity_id", s.ID, SeverityInfo, "UNBOUND_ENTITY",
						fmt.Sprintf("subject %q binds to entity %q not present in world (reality de-sync)", s.ID, b.EntityID))
				}
			}
		}

		// Episode IDs
		mindEpisodeIDs := make(map[string]bool)
		for j, ep := range m.Episodes {
			ePtr := fmt.Sprintf("%s/episodes/%d", mPtr, j)
			if ep.ID == "" {
				add(ePtr+"/id", "", SeverityError, "MISSING_ID", "episode id cannot be empty")
			} else if !slugRegex.MatchString(ep.ID) {
				add(ePtr+"/id", ep.ID, SeverityError, "INVALID_SLUG", fmt.Sprintf("episode id %q must be a slug", ep.ID))
			}
			if !slices.Contains(validPrecisions, ep.Precision) {
				add(ePtr+"/precision", ep.ID, SeverityError, "INVALID_ENUM", fmt.Sprintf("invalid precision %q; allowed: %v", ep.Precision, validPrecisions))
			}
			if mindEpisodeIDs[ep.ID] {
				add(ePtr+"/id", ep.ID, SeverityError, "DUPLICATE_ID", fmt.Sprintf("duplicate episode ID %q in mind of %s", ep.ID, m.OwnerID))
			}
			mindEpisodeIDs[ep.ID] = true

			if ep.Importance < 0.05 || ep.Importance > 1.0 {
				add(ePtr+"/importance", ep.ID, SeverityError, "OUT_OF_RANGE", "episode importance must be within [0.05, 1.0]")
			}

			st, err1 := time.Parse(time.RFC3339, ep.Start)
			et, err2 := time.Parse(time.RFC3339, ep.End)
			if err1 != nil {
				add(ePtr+"/start", ep.ID, SeverityError, "INVALID_TIME", fmt.Sprintf("invalid start time RFC 3339: %v", err1))
			}
			if err2 != nil {
				add(ePtr+"/end", ep.ID, SeverityError, "INVALID_TIME", fmt.Sprintf("invalid end time RFC 3339: %v", err2))
			}
			if err1 == nil && err2 == nil && st.After(et) {
				add(ePtr+"/start", ep.ID, SeverityError, "TIME_ORDER", "episode start time cannot be after end time")
			}

			for _, pid := range ep.ParticipantIDs {
				if !mindSubjectIDs[pid] && pid != m.OwnerID {
					add(ePtr+"/participant_ids", ep.ID, SeverityError, "UNKNOWN_SUBJECT", fmt.Sprintf("participant %q not found in mind's subjects", pid))
				}
			}
			for _, plid := range ep.PlaceIDs {
				if !mindSubjectIDs[plid] {
					add(ePtr+"/place_ids", ep.ID, SeverityError, "UNKNOWN_SUBJECT", fmt.Sprintf("place %q not found in mind's subjects", plid))
				}
			}

			if len(ep.Beats) == 0 {
				add(ePtr+"/beats", ep.ID, SeverityError, "EMPTY_BEATS", "episode must have at least one beat")
			}
			for bIdx, beat := range ep.Beats {
				bPtr := fmt.Sprintf("%s/beats/%d", ePtr, bIdx)
				if strings.TrimSpace(beat.Text) == "" {
					add(bPtr+"/text", ep.ID, SeverityError, "EMPTY_BEAT_TEXT", "beat text cannot be empty")
				}
				for _, sid := range beat.SubjectIDs {
					if !mindSubjectIDs[sid] && sid != m.OwnerID {
						add(bPtr+"/subject_ids", ep.ID, SeverityError, "UNKNOWN_SUBJECT", fmt.Sprintf("beat subject %q not in mind", sid))
					}
				}
			}
		}

		// Dossiers
		seenDossierSubjects := make(map[string]bool)
		for j, d := range m.Dossiers {
			dPtr := fmt.Sprintf("%s/dossiers/%d", mPtr, j)
			if !mindSubjectIDs[d.SubjectID] && d.SubjectID != m.OwnerID {
				add(dPtr+"/subject_id", d.SubjectID, SeverityError, "UNKNOWN_SUBJECT", fmt.Sprintf("dossier subject %q does not exist in mind", d.SubjectID))
			}
			if seenDossierSubjects[d.SubjectID] {
				add(dPtr+"/subject_id", d.SubjectID, SeverityError, "DUPLICATE_DOSSIER", fmt.Sprintf("duplicate dossier for subject %q", d.SubjectID))
			}
			seenDossierSubjects[d.SubjectID] = true

			for k, c := range d.Claims {
				cPtr := fmt.Sprintf("%s/claims/%d", dPtr, k)
				if c.ID == "" {
					add(cPtr+"/id", "", SeverityError, "MISSING_ID", "claim id cannot be empty")
				} else if !slugRegex.MatchString(c.ID) {
					add(cPtr+"/id", c.ID, SeverityError, "INVALID_SLUG", fmt.Sprintf("claim id %q must be a slug", c.ID))
				}
				if !slices.Contains(validClaimKinds, c.Kind) {
					add(cPtr+"/kind", c.ID, SeverityError, "INVALID_ENUM", fmt.Sprintf("invalid claim kind %q; allowed: %v", c.Kind, validClaimKinds))
				}
				if c.Salience < 0.05 || c.Salience > 1.0 {
					add(cPtr+"/salience", c.ID, SeverityError, "OUT_OF_RANGE", "claim salience must be within [0.05, 1.0]")
				}
				if c.Conviction < 0 || c.Conviction > 1.0 {
					add(cPtr+"/conviction", c.ID, SeverityError, "OUT_OF_RANGE", "claim conviction must be within [0.0, 1.0]")
				}
				for _, evID := range c.Evidence {
					if !mindEpisodeIDs[evID] {
						add(cPtr+"/evidence", c.ID, SeverityError, "UNKNOWN_EPISODE", fmt.Sprintf("claim evidence episode %q not in mind", evID))
					}
				}
			}
		}
	}

	// 5. Starts Validation
	if len(doc.Starts) == 0 {
		add("/starts", "", SeverityError, "NO_STARTS", "scenario must define at least one starting point")
	}

	// Map actor entities to their immediate enterable spaces
	actorSpaceMap := make(map[string]string)
	for _, e := range doc.Entities {
		if e.Kind == "actor" {
			curr := e.ParentID
			for curr != "" {
				p := entityMap[curr]
				if p.Kind == "space" && p.Space != nil && p.Space.Enterable {
					actorSpaceMap[e.ID] = p.ID
					break
				}
				curr = p.ParentID
			}
		}
	}

	startMap := make(map[string]spec.Start)
	startIDs := make(map[string]string)
	var tick1Actors []string

	for i, st := range doc.Starts {
		sPtr := fmt.Sprintf("/starts/%d", i)
		if checkSlug(st.ID, sPtr+"/id", "start") {
			if prev, exists := startIDs[st.ID]; exists {
				add(sPtr+"/id", st.ID, SeverityError, "DUPLICATE_ID", fmt.Sprintf("duplicate start ID %q (previously declared at %s)", st.ID, prev))
			} else {
				startIDs[st.ID] = sPtr
			}
		}
		startMap[st.ID] = st

		if st.Clock.TickSeconds <= 0 {
			add(sPtr+"/clock/tick_seconds", st.ID, SeverityError, "INVALID_CLOCK", "clock.tick_seconds must be positive")
		}
		if st.PlayerID != "" {
			if !actorEntities[st.PlayerID] {
				add(sPtr+"/player_id", st.ID, SeverityError, "UNKNOWN_PLAYER", fmt.Sprintf("player_id %q is not an actor entity", st.PlayerID))
			}
		}

		presentSet := make(map[string]bool)
		for _, pid := range st.PresentIDs {
			if _, ok := entityMap[pid]; !ok {
				add(sPtr+"/present_ids", st.ID, SeverityError, "UNKNOWN_PRESENT", fmt.Sprintf("present entity %q not found in entities", pid))
			}
			presentSet[pid] = true
		}

		// Present IDs closure: check ancestors and portal endpoints
		if len(st.PresentIDs) > 0 {
			for _, pid := range st.PresentIDs {
				e, ok := entityMap[pid]
				if !ok {
					continue
				}
				curr := e.ParentID
				for curr != "" {
					if !presentSet[curr] {
						add(sPtr+"/present_ids", st.ID, SeverityError, "MISSING_ANCESTOR_IN_PRESENT",
							fmt.Sprintf("present entity %q requires ancestor %q to also be in present_ids", pid, curr))
						break
					}
					curr = entityMap[curr].ParentID
				}
				if e.Kind == "portal" && e.Portal != nil {
					if !presentSet[e.Portal.FromID] {
						add(sPtr+"/present_ids", st.ID, SeverityError, "MISSING_PORTAL_ENDPOINT",
							fmt.Sprintf("portal %q requires from_id %q to be in present_ids", pid, e.Portal.FromID))
					}
					if !presentSet[e.Portal.ToID] {
						add(sPtr+"/present_ids", st.ID, SeverityError, "MISSING_PORTAL_ENDPOINT",
							fmt.Sprintf("portal %q requires to_id %q to be in present_ids", pid, e.Portal.ToID))
					}
				}
			}
		}

		// Overrides validation
		for oIdx, ov := range st.Overrides {
			oPtr := fmt.Sprintf("%s/overrides/%d", sPtr, oIdx)
			if _, ok := entityMap[ov.EntityID]; !ok {
				add(oPtr+"/entity_id", ov.EntityID, SeverityError, "UNKNOWN_OVERRIDE_TARGET",
					fmt.Sprintf("override targets unknown entity %q", ov.EntityID))
			} else if len(st.PresentIDs) > 0 && !presentSet[ov.EntityID] {
				add(oPtr+"/entity_id", ov.EntityID, SeverityError, "OVERRIDE_TARGET_NOT_PRESENT",
					fmt.Sprintf("override targets entity %q which is not in present_ids", ov.EntityID))
			}
		}

		// Mind overrides validation
		for mIdx, mo := range st.Minds {
			moPtr := fmt.Sprintf("%s/minds/%d", sPtr, mIdx)
			if _, ok := charMap[mo.OwnerID]; !ok {
				add(moPtr+"/owner_id", mo.OwnerID, SeverityError, "UNKNOWN_MIND_OWNER",
					fmt.Sprintf("start mind override references unknown character %q", mo.OwnerID))
			}
			var ownerMind *spec.Mind
			for mIdx2 := range doc.Minds {
				if doc.Minds[mIdx2].OwnerID == mo.OwnerID {
					ownerMind = &doc.Minds[mIdx2]
					break
				}
			}
			ownerSubjs := make(map[string]bool)
			ownerClaims := make(map[string]bool)
			if ownerMind != nil {
				for _, s := range ownerMind.Subjects {
					ownerSubjs[s.ID] = true
				}
				for _, d := range ownerMind.Dossiers {
					for _, c := range d.Claims {
						ownerClaims[c.ID] = true
					}
				}
			}
			for _, ups := range mo.UpsertSubjects {
				ownerSubjs[ups.ID] = true
			}
			for cIdx, up := range mo.UpsertClaims {
				cPtr := fmt.Sprintf("%s/upsert_claims/%d", moPtr, cIdx)
				if !ownerSubjs[up.SubjectID] && up.SubjectID != mo.OwnerID {
					add(cPtr+"/subject_id", up.SubjectID, SeverityError, "UNKNOWN_SUBJECT",
						fmt.Sprintf("upsert_claim subject %q not found in mind", up.SubjectID))
				}
			}
			for rIdx, retID := range mo.RetractClaimIDs {
				rPtr := fmt.Sprintf("%s/retract_claim_ids/%d", moPtr, rIdx)
				if !ownerClaims[retID] {
					add(rPtr, retID, SeverityWarning, "UNKNOWN_RETRACT_CLAIM",
						fmt.Sprintf("retract_claim_id %q does not match any existing claim in mind", retID))
				}
			}
		}

		// Check initial events
		eventIDs := make(map[string]string)
		actorsReachedInStart := make(map[string]bool)

		for j, ev := range st.Events {
			ePtr := fmt.Sprintf("%s/events/%d", sPtr, j)
			if checkSlug(ev.ID, ePtr+"/id", "event") {
				if prev, exists := eventIDs[ev.ID]; exists {
					add(ePtr+"/id", ev.ID, SeverityError, "DUPLICATE_ID", fmt.Sprintf("duplicate event ID %q in start %s (previously declared at %s)", ev.ID, st.ID, prev))
				} else {
					eventIDs[ev.ID] = ePtr
				}
			}
			if ev.Tick < 1 {
				add(ePtr+"/tick", ev.ID, SeverityError, "INVALID_TICK", "event tick must be >= 1")
			}
			if !spaceEntities[ev.LocationID] {
				add(ePtr+"/location_id", ev.ID, SeverityError, "UNKNOWN_SPACE", fmt.Sprintf("event location %q is not a space", ev.LocationID))
			}

			// Trace which actors are reached by tick 1 event
			if ev.Tick == 1 {
				for aID, spID := range actorSpaceMap {
					if spID == ev.LocationID {
						if len(st.PresentIDs) == 0 || presentSet[aID] {
							actorsReachedInStart[aID] = true
						}
					}
				}
			}

			// Authoring lint: quoted dialogue in events
			if strings.Contains(ev.Text, "\"") || strings.Contains(ev.Text, "“") {
				add(ePtr+"/text", ev.ID, SeverityWarning, "LINT_SCRIPTED_DIALOGUE",
					"opening event contains quoted dialogue; opening events should describe physical situation rather than pre-scripting speech")
			}
		}

		// Authoring lint: tick 1 activity
		hasTick1Event := false
		for _, ev := range st.Events {
			if ev.Tick == 1 {
				hasTick1Event = true
				break
			}
		}
		if !hasTick1Event {
			add(sPtr+"/events", st.ID, SeverityError, "NO_TICK1_EVENT", "start must contain at least one tick-1 event to trigger simulation wake-up")
		} else if len(actorsReachedInStart) == 0 {
			add(sPtr+"/events", st.ID, SeverityError, "NO_ACTORS_REACHED",
				"none of the tick-1 events reach any present actor in the world; simulation would stall")
		}

		if startID == "" || startID == st.ID {
			for aID := range actorsReachedInStart {
				tick1Actors = append(tick1Actors, aID)
			}
		}
	}

	if startID != "" {
		if _, ok := startMap[startID]; !ok {
			add("/starts", startID, SeverityError, "START_NOT_FOUND", fmt.Sprintf("specified start ID %q not found in scenario starts", startID))
		}
	}

	slices.Sort(tick1Actors)

	// Determine overall validity
	hasError := false
	for _, iss := range issues {
		if iss.Severity == SeverityError {
			hasError = true
			break
		}
	}

	return ValidationReport{
		Valid:                !hasError,
		Issues:               issues,
		ActorsReachedOnTick1: tick1Actors,
	}
}
