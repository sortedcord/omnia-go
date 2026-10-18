package memory

import (
	"encoding/json"
	"sort"
	"time"
)

// Snapshot captures the complete, serializable state of an actor's Mind.
type Snapshot struct {
	OwnerID          string       `json:"owner_id"`
	Turns            []Turn       `json:"turns"`
	LastConsolidated int64        `json:"last_consolidated_seq"`
	Episodes         []*Episode   `json:"episodes"`
	Subjects         []*Subject   `json:"subjects"`
	Dossiers         []*Dossier   `json:"dossiers"`
	OpenEpisodeID    string       `json:"open_episode_id,omitempty"`
	LastDecayTime    time.Time    `json:"last_decay_time"`
	Config           RecallConfig `json:"recall_config"`
	NextSubjectSeq   int          `json:"next_subject_seq"`
	NextClaimSeq     int          `json:"next_claim_seq"`
	NextEpisodeSeq   int          `json:"next_episode_seq"`
}

// ExportSnapshot packages the entire in-memory mind into a Snapshot struct.
func (m *Mind) ExportSnapshot() Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var eps []*Episode
	for _, ep := range m.episodes {
		eps = append(eps, ep)
	}
	sort.Slice(eps, func(i, j int) bool {
		return eps[i].ID < eps[j].ID
	})

	var subjs []*Subject
	for _, s := range m.subjects {
		subjs = append(subjs, s)
	}
	sort.Slice(subjs, func(i, j int) bool {
		return subjs[i].ID < subjs[j].ID
	})

	var doss []*Dossier
	for _, d := range m.dossiers {
		doss = append(doss, d)
	}
	sort.Slice(doss, func(i, j int) bool {
		return doss[i].SubjectID < doss[j].SubjectID
	})

	return Snapshot{
		OwnerID:          m.ownerID,
		Turns:            m.buffer.AllTurns(),
		LastConsolidated: m.buffer.LastConsolidatedSeq(),
		Episodes:         eps,
		Subjects:         subjs,
		Dossiers:         doss,
		OpenEpisodeID:    m.openEpisodeID,
		LastDecayTime:    m.lastDecayTime,
		Config:           m.config,
		NextSubjectSeq:   m.nextSubjectSeq,
		NextClaimSeq:     m.nextClaimSeq,
		NextEpisodeSeq:   m.nextEpisodeSeq,
	}
}

// ImportSnapshot restores an in-memory mind from a Snapshot struct.
func (m *Mind) ImportSnapshot(snap Snapshot) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.ownerID = snap.OwnerID
	m.buffer = NewWorkingBuffer(2500, 4)
	for _, t := range snap.Turns {
		m.buffer.Append(t)
	}
	m.buffer.CommitConsolidation(snap.LastConsolidated)

	m.episodes = make(map[string]*Episode)
	for _, ep := range snap.Episodes {
		m.episodes[ep.ID] = ep
	}

	m.subjects = make(map[string]*Subject)
	for _, s := range snap.Subjects {
		m.subjects[s.ID] = s
	}

	m.dossiers = make(map[string]*Dossier)
	for _, d := range snap.Dossiers {
		m.dossiers[d.SubjectID] = d
	}

	m.openEpisodeID = snap.OpenEpisodeID
	m.lastDecayTime = snap.LastDecayTime
	m.config = snap.Config
	m.nextSubjectSeq = snap.NextSubjectSeq
	m.nextClaimSeq = snap.NextClaimSeq
	m.nextEpisodeSeq = snap.NextEpisodeSeq

	if m.nextSubjectSeq == 0 {
		m.nextSubjectSeq = 1
	}
	if m.nextClaimSeq == 0 {
		m.nextClaimSeq = 1
	}
	if m.nextEpisodeSeq == 0 {
		m.nextEpisodeSeq = 1
	}
}

// MarshalJSON serializes the Mind into standard JSON.
func (m *Mind) MarshalJSON() ([]byte, error) {
	snap := m.ExportSnapshot()
	return json.Marshal(snap)
}

// UnmarshalJSON deserializes the Mind from standard JSON.
func (m *Mind) UnmarshalJSON(data []byte) error {
	var snap Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}
	m.ImportSnapshot(snap)
	return nil
}
