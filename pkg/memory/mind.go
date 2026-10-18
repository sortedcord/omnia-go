package memory

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// MindOption configures a Mind instance.
type MindOption func(*Mind)

// WithBudgetTokens overrides the working buffer token budget.
func WithBudgetTokens(tokens int) MindOption {
	return func(m *Mind) {
		m.buffer.budgetTokens = tokens
	}
}

// WithRecallConfig configures custom recall scoring parameters.
func WithRecallConfig(cfg RecallConfig) MindOption {
	return func(m *Mind) {
		m.config = cfg
	}
}

// Mind coordinates Tier 1 (Working Buffer), Tier 2 (Episodes), and Tier 3 (Dossiers) behind a single mutex.
type Mind struct {
	mu sync.RWMutex

	ownerID         string
	buffer          *WorkingBuffer
	episodes        map[string]*Episode
	subjects        map[string]*Subject
	dossiers        map[string]*Dossier
	openEpisodeID   string
	lastDecayTime   time.Time
	config          RecallConfig
	isConsolidating bool

	nextSubjectSeq int
	nextClaimSeq   int
	nextEpisodeSeq int
}

// NewMind initializes an actor's Mind, bootstrapping their self-subject and self-dossier.
func NewMind(ownerID string, opts ...MindOption) *Mind {
	m := &Mind{
		ownerID:        ownerID,
		buffer:         NewWorkingBuffer(2500, 4),
		episodes:       make(map[string]*Episode),
		subjects:       make(map[string]*Subject),
		dossiers:       make(map[string]*Dossier),
		config:         DefaultRecallConfig(),
		nextSubjectSeq: 1,
		nextClaimSeq:   1,
		nextEpisodeSeq: 1,
	}

	// Bootstrap Self-Subject and Self-Dossier
	selfSubj := &Subject{
		ID:     ownerID,
		Kind:   SubjectPerson,
		Names:  []string{"me", "myself", ownerID},
		Origin: OriginAuthored,
	}
	m.subjects[ownerID] = selfSubj

	selfDossier := &Dossier{
		SubjectID: ownerID,
	}
	m.dossiers[ownerID] = selfDossier

	for _, opt := range opts {
		opt(m)
	}

	return m
}

// OwnerID returns the actor ID owning this mind.
func (m *Mind) OwnerID() string {
	return m.ownerID
}

// Buffer returns the Tier 1 Working Buffer.
func (m *Mind) Buffer() *WorkingBuffer {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.buffer
}

// OpenEpisodeID returns the ID of the currently open episode, if any.
func (m *Mind) OpenEpisodeID() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.openEpisodeID
}

// RecordTurn appends an interaction turn to the working buffer and performs automatic percept recognition.
func (m *Mind) RecordTurn(t Turn) int64 {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Ensure turn links to open episode
	t.EpisodeID = m.openEpisodeID

	seq := m.buffer.Append(t)

	// Mechanical: auto-register clearly perceived entities (LOD >= 2) into subject registry and dossiers
	AutoRegisterPercepts(m.subjects, m.dossiers, t.Perceived, t.Tick, t.Time, func() string {
		id := fmt.Sprintf("s-%s-%d", m.ownerID, m.nextSubjectSeq)
		m.nextSubjectSeq++
		return id
	})

	return seq
}

// EnsureOpenEpisode checks if the scene changed or if an episode needs to be opened.
// If the scene changed, it closes the existing episode and opens a new one.
func (m *Mind) EnsureOpenEpisode(sceneID, locationID string, tick int, inWorldTime time.Time) (epID string, newlyOpened bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.openEpisodeID != "" {
		curEp, ok := m.episodes[m.openEpisodeID]
		if ok && curEp.Status == EpisodeOpen {
			// If scene is unchanged and time gap is <= 10 minutes, keep existing episode open
			timeGap := inWorldTime.Sub(curEp.End)
			if curEp.SceneID == sceneID && timeGap <= 10*time.Minute {
				curEp.End = inWorldTime
				if locationID != "" {
					curEp.PlaceIDs = addUniqueString(curEp.PlaceIDs, locationID)
				}
				return curEp.ID, false
			}

			// Boundary reached: close current episode
			m.closeEpisodeLocked(curEp, tick, inWorldTime)
		}
	}

	// Open new episode
	newID := fmt.Sprintf("ep-%s-%d", m.ownerID, m.nextEpisodeSeq)
	m.nextEpisodeSeq++

	newEp := &Episode{
		ID:         newID,
		OwnerID:    m.ownerID,
		Source:     SourceLived,
		Status:     EpisodeOpen,
		SceneID:    sceneID,
		PrevID:     m.openEpisodeID,
		Start:      inWorldTime,
		End:        inWorldTime,
		Precision:  PrecisionMinute,
		Title:      fmt.Sprintf("Scene %s", sceneID),
		Summary:    "In progress...",
		Importance: 0.50,
	}
	if locationID != "" {
		newEp.PlaceIDs = []string{locationID}
	}

	m.episodes[newID] = newEp
	m.openEpisodeID = newID
	return newID, true
}

func (m *Mind) closeEpisodeLocked(ep *Episode, tick int, inWorldTime time.Time) {
	ep.Status = EpisodeClosed
	ep.End = inWorldTime

	// Automatically link this episode to dossiers of recognized participants and places
	for _, partID := range ep.ParticipantIDs {
		d, ok := m.dossiers[partID]
		if !ok {
			d = &Dossier{SubjectID: partID}
			m.dossiers[partID] = d
		}
		d.AddLink(ep.ID, 0.50, RoleParticipant)
	}
	for _, placeID := range ep.PlaceIDs {
		d, ok := m.dossiers[placeID]
		if !ok {
			d = &Dossier{SubjectID: placeID}
			m.dossiers[placeID] = d
		}
		d.AddLink(ep.ID, 0.50, RolePlace)
	}
}

// FinalizeEpisodeClose commits the consolidator's title, summary, tone, and importance to a closed episode.
func (m *Mind) FinalizeEpisodeClose(epID string, title, summary, tone string, importance float64, formativeSubjectIDs []string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	ep, ok := m.episodes[epID]
	if !ok {
		return
	}
	if title != "" {
		ep.Title = strings.TrimSpace(title)
	}
	if summary != "" {
		ep.Summary = strings.TrimSpace(summary)
	}
	if tone != "" {
		ep.Tone = strings.TrimSpace(tone)
	}
	if importance >= 0.05 && importance <= 1.00 {
		ep.Importance = importance
	}

	// Raise formative links
	for _, subjID := range formativeSubjectIDs {
		if d, ok := m.dossiers[subjID]; ok {
			d.AddLink(ep.ID, 1.00, RoleFormative)
		}
	}
}

// AppendBeat appends a condensed beat to the specified episode.
func (m *Mind) AppendBeat(epID string, beat Beat) {
	m.mu.Lock()
	defer m.mu.Unlock()

	ep, ok := m.episodes[epID]
	if !ok {
		return
	}
	ep.Beats = append(ep.Beats, beat)
	for _, subjID := range beat.SubjectIDs {
		ep.ParticipantIDs = addUniqueString(ep.ParticipantIDs, subjID)
	}
}

// ApplyDossierOps validates and executes atomic thought mutations from the Distiller.
func (m *Mind) ApplyDossierOps(tick int, ops []DossierOp, inWorldTime time.Time) (applied []DossierOp, rejected []string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Apply decay to all dossiers before mutations
	if !m.lastDecayTime.IsZero() && inWorldTime.After(m.lastDecayTime) {
		for _, d := range m.dossiers {
			d.DecayClaims(inWorldTime, m.lastDecayTime)
		}
	}
	m.lastDecayTime = inWorldTime

	for _, op := range ops {
		switch op.Op {
		case "new_subject":
			if op.Name == "" {
				rejected = append(rejected, "new_subject: name cannot be empty")
				continue
			}
			id := op.SubjectID
			if id == "" {
				id = fmt.Sprintf("s-%s-%d", m.ownerID, m.nextSubjectSeq)
				m.nextSubjectSeq++
			}
			kind := op.SubjectKind
			if kind == "" {
				kind = SubjectPerson
			}
			subj := &Subject{
				ID:     id,
				Kind:   kind,
				Names:  []string{strings.TrimSpace(op.Name)},
				Origin: OriginPerceived,
			}
			m.subjects[id] = subj
			if _, ok := m.dossiers[id]; !ok {
				m.dossiers[id] = &Dossier{SubjectID: id}
			}
			applied = append(applied, op)

		case "add_name":
			subj, ok := m.subjects[op.SubjectID]
			if !ok {
				rejected = append(rejected, fmt.Sprintf("add_name: subject %q not found", op.SubjectID))
				continue
			}
			name := strings.TrimSpace(op.Name)
			if name != "" {
				subj.Names = addUniqueString(subj.Names, name)
				applied = append(applied, op)
			}

		case "bind":
			subj, ok := m.subjects[op.SubjectID]
			if !ok {
				rejected = append(rejected, fmt.Sprintf("bind: subject %q not found", op.SubjectID))
				continue
			}
			mode := op.BindMode
			if mode == "" {
				mode = BindingIdentity
			}
			subj.Bindings = append(subj.Bindings, Binding{
				EntityID:          op.EntityID,
				Mode:              mode,
				Confidence:        1.0,
				LastConfirmedTick: tick,
				LastConfirmedAt:   inWorldTime,
			})
			applied = append(applied, op)

		case "unbind":
			subj, ok := m.subjects[op.SubjectID]
			if !ok {
				rejected = append(rejected, fmt.Sprintf("unbind: subject %q not found", op.SubjectID))
				continue
			}
			var filtered []Binding
			for _, b := range subj.Bindings {
				if b.EntityID != op.EntityID {
					filtered = append(filtered, b)
				}
			}
			subj.Bindings = filtered
			applied = append(applied, op)

		case "upsert_claim", "retract_claim", "set_impression":
			subjID := op.SubjectID
			if subjID == "" {
				subjID = m.ownerID // Defaults to self
			}
			d, ok := m.dossiers[subjID]
			if !ok {
				d = &Dossier{SubjectID: subjID}
				m.dossiers[subjID] = d
			}
			// Allocate ClaimID if needed
			if op.Op == "upsert_claim" && op.ClaimID == "" {
				op.ClaimID = fmt.Sprintf("c-%s-%d", m.ownerID, m.nextClaimSeq)
				m.nextClaimSeq++
			}

			if err := d.ApplyOp(tick, op, m.openEpisodeID); err != nil {
				rejected = append(rejected, err.Error())
				continue
			}
			applied = append(applied, op)
		}
	}

	return applied, rejected
}

// AddSubject inserts a subject directly (used for seeding scenarios).
func (m *Mind) AddSubject(s *Subject) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.subjects[s.ID] = s
	if _, ok := m.dossiers[s.ID]; !ok {
		m.dossiers[s.ID] = &Dossier{SubjectID: s.ID}
	}
}

// AddClaim inserts or updates a claim directly (used for seeding scenarios).
func (m *Mind) AddClaim(subjectID string, c Claim) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if subjectID == "" {
		subjectID = m.ownerID
	}
	d, ok := m.dossiers[subjectID]
	if !ok {
		d = &Dossier{SubjectID: subjectID}
		m.dossiers[subjectID] = d
	}

	if c.ID == "" {
		c.ID = fmt.Sprintf("c-%s-%d", m.ownerID, m.nextClaimSeq)
		m.nextClaimSeq++
	}

	for i := range d.Claims {
		if d.Claims[i].ID == c.ID {
			d.Claims[i] = c
			return
		}
	}
	d.Claims = append(d.Claims, c)
}

// AddEpisode seeds an authored episode directly into the archive.
func (m *Mind) AddEpisode(ep *Episode) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.episodes[ep.ID] = ep
}

// Recall performs memory scoring and returns the retrieval results.
func (m *Mind) Recall(cues RecallCues) RecallResult {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Apply decay
	if !m.lastDecayTime.IsZero() && cues.CurrentTime.After(m.lastDecayTime) {
		for _, d := range m.dossiers {
			d.DecayClaims(cues.CurrentTime, m.lastDecayTime)
		}
	}
	m.lastDecayTime = cues.CurrentTime

	return ExecuteRecall(
		m.ownerID,
		m.subjects,
		m.dossiers,
		m.episodes,
		m.openEpisodeID,
		cues,
		m.config,
	)
}

// RenderContext generates the developer context sections for Self, Dossiers, and Memories within budgets.
func (m *Mind) RenderContext(cues RecallCues) (selfText, dossiersText, memoriesText string) {
	recallRes := m.Recall(cues)
	return RenderPromptSections(recallRes, m.subjects, 300, 700, 900)
}

// CloseAllOpenEpisodes closes any open episodes (called when simulation finishes).
func (m *Mind) CloseAllOpenEpisodes(tick int, inWorldTime time.Time) []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	var closed []string
	if m.openEpisodeID != "" {
		if curEp, ok := m.episodes[m.openEpisodeID]; ok && curEp.Status == EpisodeOpen {
			m.closeEpisodeLocked(curEp, tick, inWorldTime)
			closed = append(closed, curEp.ID)
		}
		m.openEpisodeID = ""
	}
	return closed
}

// ConsolidateJob packages a chunk of turns ready for background consolidation.
type ConsolidateJob struct {
	EpisodeID string
	FromSeq   int64
	ToSeq     int64
	Turns     []Turn
}

// StartConsolidation atomically marks the buffer as consolidating and returns uncommitted turns.
func (m *Mind) StartConsolidation() (ConsolidateJob, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.isConsolidating || !m.buffer.NeedsConsolidation() {
		return ConsolidateJob{}, false
	}

	unconsolidated := m.buffer.UnconsolidatedTurns()
	if len(unconsolidated) == 0 {
		return ConsolidateJob{}, false
	}

	m.isConsolidating = true
	return ConsolidateJob{
		EpisodeID: m.openEpisodeID,
		FromSeq:   unconsolidated[0].Seq,
		ToSeq:     unconsolidated[len(unconsolidated)-1].Seq,
		Turns:     unconsolidated,
	}, true
}

// FinishConsolidation appends consolidated beats and advances the buffer watermark.
func (m *Mind) FinishConsolidation(job ConsolidateJob, beats []Beat) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.isConsolidating = false
	if len(beats) > 0 && job.EpisodeID != "" {
		if ep, ok := m.episodes[job.EpisodeID]; ok {
			ep.Beats = append(ep.Beats, beats...)
		}
	}
	m.buffer.CommitConsolidation(job.ToSeq)
}

// AbortConsolidation resets the consolidation flag and triggers degraded fallback only if at 2x budget.
func (m *Mind) AbortConsolidation() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.isConsolidating = false
	if m.buffer.EstimatedTokens() > 2*m.buffer.budgetTokens {
		_, beatText, fSeq, tSeq := m.buffer.DegradedEvictionFallback()
		if beatText != "" && m.openEpisodeID != "" {
			if ep, ok := m.episodes[m.openEpisodeID]; ok {
				ep.Beats = append(ep.Beats, Beat{
					FromSeq:  fSeq,
					ToSeq:    tSeq,
					Text:     beatText,
					Degraded: true,
				})
			}
		}
	}
}

// GetSubject returns a registered subject by ID.
func (m *Mind) GetSubject(id string) (*Subject, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.subjects[id]
	return s, ok
}
func (m *Mind) GetDossier(subjectID string) (*Dossier, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.dossiers[subjectID]
	if !ok {
		d = &Dossier{SubjectID: subjectID}
		m.dossiers[subjectID] = d
		return d, false
	}
	return d, true
}

// DossiersSnapshot returns a copy of all dossiers.
func (m *Mind) DossiersSnapshot() []*Dossier {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make([]*Dossier, 0, len(m.dossiers))
	for _, d := range m.dossiers {
		cp := *d
		res = append(res, &cp)
	}
	return res
}

// EpisodesSnapshot returns a copy of all episodes.
func (m *Mind) EpisodesSnapshot() []*Episode {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make([]*Episode, 0, len(m.episodes))
	for _, ep := range m.episodes {
		cp := *ep
		res = append(res, &cp)
	}
	return res
}

// SubjectsSnapshot returns a copy of all subjects.
func (m *Mind) SubjectsSnapshot() []*Subject {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make([]*Subject, 0, len(m.subjects))
	for _, s := range m.subjects {
		cp := *s
		res = append(res, &cp)
	}
	return res
}

// SubjectsSnapshotMap returns a copy of all subjects keyed by ID.
func (m *Mind) SubjectsSnapshotMap() map[string]*Subject {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make(map[string]*Subject, len(m.subjects))
	for k, v := range m.subjects {
		cp := *v
		res[k] = &cp
	}
	return res
}
