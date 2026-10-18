package memory

import (
	"math"
	"sort"
	"strings"
	"time"
)

// RecallConfig tunes memory activation, episodic scoring, and surfacing thresholds.
type RecallConfig struct {
	MinEpisodeScore           float64       `json:"min_episode_score"`           // θ = 0.35
	VividEpisodeScore         float64       `json:"vivid_episode_score"`         // 0.80: renders full beats
	MaxActivatedDossiers      int           `json:"max_activated_dossiers"`      // Top 4 non-self dossiers
	MaxSurfacedEpisodes       int           `json:"max_surfaced_episodes"`       // Top 3 episodes
	RefractoryTicks           int           `json:"refractory_ticks"`            // 3 ticks
	RefractoryPenalty         float64       `json:"refractory_penalty"`          // 0.50
	SelfActivationBaseline    float64       `json:"self_activation_baseline"`    // 0.40
	NamedMentionActivation    float64       `json:"named_mention_activation"`    // 0.90
	ExpectationMissActivation float64       `json:"expectation_miss_activation"` // 0.90
	AnchoredActivation        float64       `json:"anchored_activation"`         // 0.50
	RecencyTimeScale          time.Duration `json:"recency_time_scale"`          // 24 hours
}

// DefaultRecallConfig provides calibrated baselines matching the memory specification.
func DefaultRecallConfig() RecallConfig {
	return RecallConfig{
		MinEpisodeScore:           0.25,
		VividEpisodeScore:         0.75,
		MaxActivatedDossiers:      4,
		MaxSurfacedEpisodes:       3,
		RefractoryTicks:           3,
		RefractoryPenalty:         0.50,
		SelfActivationBaseline:    0.40,
		NamedMentionActivation:    0.90,
		ExpectationMissActivation: 0.90,
		AnchoredActivation:        0.50,
		RecencyTimeScale:          24 * time.Hour,
	}
}

// RecallCues packages all sensory and attentional signals driving memory retrieval this tick.
type RecallCues struct {
	Percepts      []Percept          // Entities in current perception with LOD
	Mentions      []string           // Explicit names/aliases mentioned in new sensory text
	FocusMap      map[string]float64 // EntityID -> attention boost
	LocationChain []string           // Current space and its ancestors
	CurrentSpace  string             // Immediate space ID
	CurrentTick   int
	CurrentTime   time.Time
}

// EpisodeScoreBreakdown provides full explainability for each scored episodic memory.
type EpisodeScoreBreakdown struct {
	EpisodeID         string  `json:"episode_id"`
	TotalScore        float64 `json:"total_score"`
	CueStrength       float64 `json:"cue_strength"`
	Importance        float64 `json:"importance"`
	Accessibility     float64 `json:"accessibility"`
	ContextMatch      float64 `json:"context_match"`
	RefractoryPenalty float64 `json:"refractory_penalty"`
	Surfaced          bool    `json:"surfaced"`
	Vivid             bool    `json:"vivid"`
}

// RecallResult packages the final state of memory activation and retrieval.
type RecallResult struct {
	SelfDossier       *Dossier
	ActivatedDossiers []*Dossier
	SurfacedEpisodes  []*Episode
	VividEpisodes     map[string]bool
	ScoreBreakdowns   map[string]EpisodeScoreBreakdown
	ExpectationMisses []string // Subject IDs (implicit internal signal)
}

// ExecuteRecall performs pure, deterministic memory scoring and retrieval.
func ExecuteRecall(
	ownerID string,
	subjects map[string]*Subject,
	dossiers map[string]*Dossier,
	episodes map[string]*Episode,
	openEpisodeID string,
	cues RecallCues,
	cfg RecallConfig,
) RecallResult {
	res := RecallResult{
		VividEpisodes:   make(map[string]bool),
		ScoreBreakdowns: make(map[string]EpisodeScoreBreakdown),
	}

	// 1. Recognize percepts and detect expectation misses
	recog := Recognize(subjects, cues.Percepts, cues.LocationChain, cues.CurrentTick, cues.CurrentTime)
	for missSubj := range recog.ExpectationMiss {
		res.ExpectationMisses = append(res.ExpectationMisses, missSubj)
	}

	// 2. Activate Dossiers
	type activatedDossier struct {
		dossier    *Dossier
		activation float64
		score      float64
	}

	mentionSet := make(map[string]bool)
	for _, m := range cues.Mentions {
		trimmed := strings.Trim(strings.ToLower(m), `"'.,?!:;()[]{}*`)
		if trimmed != "" {
			mentionSet[trimmed] = true
		}
	}

	var activations []activatedDossier

	for subjID, d := range dossiers {
		if subjID == ownerID {
			res.SelfDossier = d
			continue
		}

		subj, hasSubj := subjects[subjID]
		activation := 0.0

		// A. Recognition from perception (LOD3: 1.0, LOD2: 0.6, LOD1: 0.3)
		if conf, recognized := recog.Recognized[subjID]; recognized {
			lodScore := 0.6
			// Look up LOD from percept
			if hasSubj {
				for _, b := range subj.Bindings {
					if b.Mode == BindingIdentity {
						for _, p := range cues.Percepts {
							if p.EntityID == b.EntityID {
								switch p.LOD {
								case 3:
									lodScore = 1.0
								case 2:
									lodScore = 0.6
								case 1:
									lodScore = 0.3
								}
							}
						}
					}
				}
			}
			recScore := lodScore * conf
			if recScore > activation {
				activation = recScore
			}
		}

		// B. Named mention in sensory ingress (subject's names/aliases)
		if hasSubj {
			for _, name := range subj.Names {
				if mentionSet[strings.ToLower(name)] {
					if cfg.NamedMentionActivation > activation {
						activation = cfg.NamedMentionActivation
					}
					break
				}
			}
		}

		// C. Expectation miss cue (anchored but not seen!)
		if recog.ExpectationMiss[subjID] {
			if cfg.ExpectationMissActivation > activation {
				activation = cfg.ExpectationMissActivation
			}
		}

		// D. Anchored subject (believed to be in current space chain)
		if _, anchored := recog.Anchored[subjID]; anchored {
			if cfg.AnchoredActivation > activation {
				activation = cfg.AnchoredActivation
			}
		}

		// E. Focus map boost
		if hasSubj {
			for _, b := range subj.Bindings {
				if boost, focused := cues.FocusMap[b.EntityID]; focused {
					activation += boost
					if activation > 1.0 {
						activation = 1.0
					}
				}
			}
		}

		if activation <= 0.05 {
			continue
		}

		maxClaimSalience := 0.5
		for _, c := range d.Claims {
			if c.Salience > maxClaimSalience {
				maxClaimSalience = c.Salience
			}
		}

		dScore := activation * (0.5 + 0.5*maxClaimSalience)
		activations = append(activations, activatedDossier{
			dossier:    d,
			activation: activation,
			score:      dScore,
		})
	}

	// Sort dossiers by score descending
	sort.Slice(activations, func(i, j int) bool {
		return activations[i].score > activations[j].score
	})

	limitDossiers := cfg.MaxActivatedDossiers
	if len(activations) < limitDossiers {
		limitDossiers = len(activations)
	}

	selectedDossiers := make(map[string]activatedDossier)
	for i := 0; i < limitDossiers; i++ {
		res.ActivatedDossiers = append(res.ActivatedDossiers, activations[i].dossier)
		selectedDossiers[activations[i].dossier.SubjectID] = activations[i]
	}

	// 3. Score Linked Episodes
	type candidateEp struct {
		ep        *Episode
		breakdown EpisodeScoreBreakdown
	}

	var candidates []candidateEp

	for epID, ep := range episodes {
		// Open episode is excluded (its content is already in working buffer)
		if epID == openEpisodeID || ep.Status == EpisodeOpen {
			continue
		}

		// Compute cue strength from selected dossiers
		cue := 0.0
		for _, ad := range selectedDossiers {
			for _, link := range ad.dossier.Links {
				if link.EpisodeID == epID {
					cue += ad.activation * link.Weight
				}
			}
		}

		if cue <= 0.01 {
			continue // No activation path through active dossiers
		}

		// Accessibility: recency power-law decay + recall reinforcement
		elapsedHours := 0.0
		if !cues.CurrentTime.IsZero() && !ep.End.IsZero() && cues.CurrentTime.After(ep.End) {
			elapsedHours = cues.CurrentTime.Sub(ep.End).Hours()
		}

		tauHours := cfg.RecencyTimeScale.Hours()
		if tauHours <= 0 {
			tauHours = 24.0
		}
		timeDecay := math.Pow(1.0+(elapsedHours/tauHours), -0.5)
		recallBoost := 1.0 + 0.15*math.Log(1.0+float64(ep.RecallCount))
		access := timeDecay * recallBoost

		// Formative memories (importance >= 0.8) have a permanent access floor of 0.35
		floor := 0.0
		if ep.Importance >= 0.80 {
			floor = 0.35
		}
		if access < floor {
			access = floor
		}

		// Context match: participant overlap + location match
		partOverlap := computeJaccard(ep.ParticipantIDs, getRecognizedSubjectIDs(recog.Recognized))
		locMatch := 0.0
		for _, pl := range ep.PlaceIDs {
			if pl == cues.CurrentSpace || recog.Anchored[pl] > 0 {
				locMatch = 1.0
				break
			}
		}
		contextMatch := 0.5*partOverlap + 0.5*locMatch

		// Refractory penalty (suppress memories surfaced recently unless cued by a fresh mention or expectation miss)
		refract := 0.0
		if ep.LastRecalledTick > 0 && (cues.CurrentTick-ep.LastRecalledTick) <= cfg.RefractoryTicks {
			hasFreshCue := false
			for subjID := range selectedDossiers {
				if recog.ExpectationMiss[subjID] {
					hasFreshCue = true
					break
				}
				if subj, ok := subjects[subjID]; ok {
					for _, name := range subj.Names {
						if mentionSet[strings.ToLower(name)] {
							hasFreshCue = true
							break
						}
					}
				}
			}
			if !hasFreshCue {
				refract = cfg.RefractoryPenalty
			}
		}

		totalScore := cue*(0.4+0.6*ep.Importance)*access + 0.2*contextMatch - refract
		if totalScore < 0 {
			totalScore = 0
		}

		bd := EpisodeScoreBreakdown{
			EpisodeID:         epID,
			TotalScore:        totalScore,
			CueStrength:       cue,
			Importance:        ep.Importance,
			Accessibility:     access,
			ContextMatch:      contextMatch,
			RefractoryPenalty: refract,
		}

		res.ScoreBreakdowns[epID] = bd
		if totalScore >= cfg.MinEpisodeScore {
			candidates = append(candidates, candidateEp{ep: ep, breakdown: bd})
		}
	}

	// Sort candidates descending by total score
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].breakdown.TotalScore > candidates[j].breakdown.TotalScore
	})

	limitEps := cfg.MaxSurfacedEpisodes
	if len(candidates) < limitEps {
		limitEps = len(candidates)
	}

	for i := 0; i < limitEps; i++ {
		ep := candidates[i].ep
		bd := candidates[i].breakdown
		bd.Surfaced = true

		if bd.TotalScore >= cfg.VividEpisodeScore {
			bd.Vivid = true
			res.VividEpisodes[ep.ID] = true
		}

		res.SurfacedEpisodes = append(res.SurfacedEpisodes, ep)
		res.ScoreBreakdowns[ep.ID] = bd

		// Update recall bookkeeping on surfaced episode
		ep.RecallCount++
		ep.LastRecalledTick = cues.CurrentTick
		ep.LastRecalledAt = cues.CurrentTime
	}

	return res
}

func computeJaccard(a, b []string) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 0.0
	}
	set := make(map[string]bool)
	for _, x := range a {
		set[x] = true
	}
	intersect := 0
	union := len(set)
	for _, y := range b {
		if set[y] {
			intersect++
		} else {
			union++
		}
	}
	if union == 0 {
		return 0.0
	}
	return float64(intersect) / float64(union)
}

func getRecognizedSubjectIDs(m map[string]float64) []string {
	var ids []string
	for id := range m {
		ids = append(ids, id)
	}
	return ids
}
