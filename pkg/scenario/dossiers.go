package scenario

import (
	"time"

	"omnia.adityagupta.dev/pkg/actor"
	"omnia.adityagupta.dev/pkg/memory"
)

// SeedDossiers initializes Tier 3 subjective reality dossiers for all three actors.
func SeedDossiers(zack, nelly, mia *actor.Actor) {
	now := time.Date(2026, 10, 16, 18, 35, 0, 0, time.UTC)

	// =========================================================================
	// 1. NELLY'S SUBJECTIVE REALITY DOSSIERS
	// =========================================================================
	if nelly != nil && nelly.Mind != nil {
		// Self-Dossier
		nelly.Mind.AddClaim("nelly", memory.Claim{
			ID:         "c-nel-self-1",
			Kind:       memory.ClaimIntention,
			Content:    "Try not to faint or sound completely insane when Mia brings this guy inside.",
			Salience:   0.95,
			Conviction: 0.85,
			Formed:     1,
		})
		nelly.Mind.AddClaim("nelly", memory.Claim{
			ID:         "c-nel-self-2",
			Kind:       memory.ClaimFeeling,
			Content:    "My heart is hammering at 125 bpm listening to footsteps and muffled voices outside the door.",
			Salience:   0.88,
			Conviction: 0.90,
			Formed:     1,
		})
		nelly.Mind.AddClaim("nelly", memory.Claim{
			ID:         "c-nel-self-3",
			Kind:       memory.ClaimFeeling,
			Content:    "Desperately terrified of having to move back into my parents' basement in utter disgrace if I lose this lease.",
			Salience:   0.98,
			Conviction: 0.95,
			Fixed:      true,
			Evidence:   []string{"ep-nelly-audit-letter"},
			Formed:     1,
		})
		nelly.Mind.AddClaim("nelly", memory.Claim{
			ID:         "c-nel-self-4",
			Kind:       memory.ClaimIntention,
			Content:    "Get Zack to agree to stay for the Thursday inspection without making him feel like an exhibit in a museum.",
			Salience:   0.92,
			Conviction: 0.88,
			Formed:     1,
		})

		// Phantom Subject: David (Runaway Ex)
		nelly.Mind.AddSubject(&memory.Subject{
			ID:     "david",
			Kind:   memory.SubjectPerson,
			Names:  []string{"David", "ex-boyfriend"},
			Origin: memory.OriginAuthored,
		})
		nelly.Mind.AddClaim("david", memory.Claim{
			ID:         "c-nel-dav-1",
			Kind:       memory.ClaimBelief,
			Content:    "David used me for cheap subsidized rent, dumped me the second things got difficult, and left me holding the penalty.",
			Salience:   0.88,
			Conviction: 0.98,
			Fixed:      true,
			Evidence:   []string{"ep-nelly-david-abandonment"},
			Formed:     1,
		})
		nelly.Mind.AddClaim("david", memory.Claim{
			ID:         "c-nel-dav-2",
			Kind:       memory.ClaimFeeling,
			Content:    "A suffocating mixture of betrayal and humiliated grief whenever I see the empty hangers in the bedroom closet.",
			Salience:   0.75,
			Conviction: 0.90,
			Evidence:   []string{"ep-nelly-david-abandonment"},
			Formed:     1,
		})
		nelly.Mind.AddClaim("david", memory.Claim{
			ID:         "c-nel-dav-3",
			Kind:       memory.ClaimBelief,
			Content:    "If housing discovers David moved out six weeks ago, I owe $6,800 in retroactive market-rate back charges.",
			Salience:   0.95,
			Conviction: 1.00,
			Fixed:      true,
			Evidence:   []string{"ep-nelly-audit-letter"},
			Formed:     1,
		})

		// Phantom Subject: Housing Authority
		nelly.Mind.AddSubject(&memory.Subject{
			ID:     "housing-authority",
			Kind:   memory.SubjectGroup,
			Names:  []string{"University Housing Compliance Board", "Housing Inspector", "Inspector Henderson"},
			Origin: memory.OriginAuthored,
		})
		nelly.Mind.AddClaim("housing-authority", memory.Claim{
			ID:         "c-nel-ha-1",
			Kind:       memory.ClaimBelief,
			Content:    "The housing board conducts ruthless in-person checks and will revoke my lease on the spot if they suspect fraud.",
			Salience:   0.92,
			Conviction: 0.95,
			Fixed:      true,
			Evidence:   []string{"ep-nelly-audit-letter"},
			Formed:     1,
		})

		// Phantom Subject: Nelly's Parents
		nelly.Mind.AddSubject(&memory.Subject{
			ID:     "nellys-parents",
			Kind:   memory.SubjectGroup,
			Names:  []string{"Mom and Dad", "Parents"},
			Origin: memory.OriginAuthored,
		})
		nelly.Mind.AddClaim("nellys-parents", memory.Claim{
			ID:         "c-nel-par-1",
			Kind:       memory.ClaimBelief,
			Content:    "If I lose this apartment, my parents will make me move back home to interrogate me about my life choices until I dissolve into dust.",
			Salience:   0.90,
			Conviction: 0.95,
			Fixed:      true,
			Formed:     1,
		})

		// Subject: Zack (Complete Stranger)
		nelly.Mind.AddSubject(&memory.Subject{
			ID:    "zack",
			Kind:  memory.SubjectPerson,
			Names: []string{"Zack", "Mia's friend", "Stand-in boyfriend"},
			Bindings: []memory.Binding{
				{
					EntityID:          "zack",
					Mode:              memory.BindingIdentity,
					Confidence:        1.0,
					LastConfirmedTick: 1,
					LastConfirmedAt:   now,
				},
			},
			Origin: memory.OriginPerceived,
		})
		nelly.Mind.AddClaim("zack", memory.Claim{
			ID:         "c-nel-zack-1",
			Kind:       memory.ClaimBelief,
			Content:    "Zack is a complete stranger who looks tired, tall, and mildly bewildered. Mia dragged him here out of nowhere.",
			Salience:   0.88,
			Conviction: 0.90,
			Formed:     1,
		})
		nelly.Mind.AddClaim("zack", memory.Claim{
			ID:         "c-nel-zack-2",
			Kind:       memory.ClaimSuspicion,
			Content:    "Zack probably finds me completely pathetic and unhinged for making color-coded relationship notes.",
			Salience:   0.92,
			Conviction: 0.70,
			Formed:     1,
		})
		nelly.Mind.AddClaim("zack", memory.Claim{
			ID:         "c-nel-zack-3",
			Kind:       memory.ClaimIntention,
			Content:    "Offer to buy him coffee for a month so he knows I'm not taking advantage of him.",
			Salience:   0.85,
			Conviction: 0.85,
			Formed:     1,
		})

		// Subject: Mia (Best Friend)
		nelly.Mind.AddSubject(&memory.Subject{
			ID:    "mia",
			Kind:  memory.SubjectPerson,
			Names: []string{"Mia", "Best friend"},
			Bindings: []memory.Binding{
				{
					EntityID:          "mia",
					Mode:              memory.BindingIdentity,
					Confidence:        1.0,
					LastConfirmedTick: 1,
					LastConfirmedAt:   now,
				},
			},
			Origin: memory.OriginPerceived,
		})
		nelly.Mind.AddClaim("mia", memory.Claim{
			ID:         "c-nel-mia-1",
			Kind:       memory.ClaimBelief,
			Content:    "Mia is totally shameless and chaotic, but she's the only reason I didn't pack my bags in tears yesterday.",
			Salience:   0.85,
			Conviction: 0.95,
			Fixed:      true,
			Evidence:   []string{"ep-nelly-mia-scheme"},
			Formed:     1,
		})
		nelly.Mind.AddClaim("mia", memory.Claim{
			ID:         "c-nel-mia-2",
			Kind:       memory.ClaimSuspicion,
			Content:    "Mia is outside the door right now briefing this guy; I pray she isn't making me sound completely unhinged.",
			Salience:   0.82,
			Conviction: 0.85,
			Formed:     1,
		})
	}

	// =========================================================================
	// 2. ZACK'S SUBJECTIVE REALITY DOSSIERS
	// =========================================================================
	if zack != nil && zack.Mind != nil {
		// Self-Dossier
		zack.Mind.AddClaim("zack", memory.Claim{
			ID:         "c-zk-self-1",
			Kind:       memory.ClaimIntention,
			Content:    "Find out what impossible scheme Mia has cooked up before she talks me into stepping through that door.",
			Salience:   0.92,
			Conviction: 0.85,
			Formed:     1,
		})
		zack.Mind.AddClaim("zack", memory.Claim{
			ID:         "c-zk-self-2",
			Kind:       memory.ClaimFeeling,
			Content:    "Bone-tired from months of sleeping in internet cafes and benches, but stubbornly determined not to show it.",
			Salience:   0.85,
			Conviction: 0.95,
			Fixed:      true,
			Evidence:   []string{"ep-zack-cybernet-cafe", "ep-zack-lecture-humiliation"},
			Formed:     1,
		})
		zack.Mind.AddClaim("zack", memory.Claim{
			ID:         "c-zk-self-3",
			Kind:       memory.ClaimBelief,
			Content:    "I would rather freeze on a bench than be anyone's pity case, but if this is an actual practical trade, I need to hear the terms.",
			Salience:   0.90,
			Conviction: 0.92,
			Fixed:      true,
			Evidence:   []string{"ep-zack-couch-surfing"},
			Formed:     1,
		})
		zack.Mind.AddClaim("zack", memory.Claim{
			ID:         "c-zk-self-4",
			Kind:       memory.ClaimIntention,
			Content:    "Hear Mia out outside the door, but keep my guard up—I'm not agreeing to anything fraudulent or humiliating.",
			Salience:   0.85,
			Conviction: 0.80,
			Formed:     1,
		})

		// Subject: Nelly (Complete Stranger)
		zack.Mind.AddSubject(&memory.Subject{
			ID:    "nelly",
			Kind:  memory.SubjectPerson,
			Names: []string{"Nelly", "Mia's friend", "Apartment tenant"},
			Bindings: []memory.Binding{
				{
					EntityID:          "nelly",
					Mode:              memory.BindingIdentity,
					Confidence:        1.0,
					LastConfirmedTick: 1,
					LastConfirmedAt:   now,
				},
			},
			Origin: memory.OriginPerceived,
		})
		zack.Mind.AddClaim("nelly", memory.Claim{
			ID:         "c-zk-nel-1",
			Kind:       memory.ClaimBelief,
			Content:    "Nelly is a complete stranger. She looks like she's about three seconds away from spontaneous social combustion.",
			Salience:   0.92,
			Conviction: 0.90,
			Formed:     1,
		})
		zack.Mind.AddClaim("nelly", memory.Claim{
			ID:         "c-zk-nel-2",
			Kind:       memory.ClaimBelief,
			Content:    "She wrote color-coded index cards for a fake relationship backstory; she's clearly desperate and detail-obsessed.",
			Salience:   0.88,
			Conviction: 0.85,
			Formed:     1,
		})
		zack.Mind.AddClaim("nelly", memory.Claim{
			ID:         "c-zk-nel-3",
			Kind:       memory.ClaimFeeling,
			Content:    "I recognize real panic when someone's living situation is falling apart; I can't just mock her.",
			Salience:   0.85,
			Conviction: 0.90,
			Fixed:      true,
			Formed:     1,
		})

		// Subject: Mia (Chaotic Friend)
		zack.Mind.AddSubject(&memory.Subject{
			ID:    "mia",
			Kind:  memory.SubjectPerson,
			Names: []string{"Mia", "Chaotic instigator"},
			Bindings: []memory.Binding{
				{
					EntityID:          "mia",
					Mode:              memory.BindingIdentity,
					Confidence:        1.0,
					LastConfirmedTick: 1,
					LastConfirmedAt:   now,
				},
			},
			Origin: memory.OriginPerceived,
		})
		zack.Mind.AddClaim("mia", memory.Claim{
			ID:         "c-zk-mia-1",
			Kind:       memory.ClaimBelief,
			Content:    "Mia is brilliant, completely unhinged, and always drags me into trouble, but she has never actually steered me wrong.",
			Salience:   0.85,
			Conviction: 0.95,
			Fixed:      true,
			Evidence:   []string{"ep-zack-mia-ambush"},
			Formed:     1,
		})
		zack.Mind.AddClaim("mia", memory.Claim{
			ID:         "c-zk-mia-2",
			Kind:       memory.ClaimSuspicion,
			Content:    "Mia is looking at me like she's about to sell me a timeshare; I need to hear the actual catch.",
			Salience:   0.85,
			Conviction: 0.85,
			Formed:     1,
		})

		// Phantom Subject: Professor Harrison
		zack.Mind.AddSubject(&memory.Subject{
			ID:     "professor-harrison",
			Kind:   memory.SubjectPerson,
			Names:  []string{"Professor Harrison", "Harrison"},
			Origin: memory.OriginAuthored,
		})
		zack.Mind.AddClaim("professor-harrison", memory.Claim{
			ID:         "c-zk-har-1",
			Kind:       memory.ClaimBelief,
			Content:    "Harrison publicly called me out in front of 120 people; I cannot afford to fail Structural Dynamics or fall asleep in his class again.",
			Salience:   0.80,
			Conviction: 0.95,
			Fixed:      true,
			Evidence:   []string{"ep-zack-lecture-humiliation"},
			Formed:     1,
		})
	}

	// =========================================================================
	// 3. MIA'S SUBJECTIVE REALITY DOSSIERS
	// =========================================================================
	if mia != nil && mia.Mind != nil {
		// Self-Dossier
		mia.Mind.AddClaim("mia", memory.Claim{
			ID:         "c-mia-self-1",
			Kind:       memory.ClaimIntention,
			Content:    "Brief Zack outside the door and make sure he doesn't bolt before we step inside.",
			Salience:   0.95,
			Conviction: 0.90,
			Evidence:   []string{"ep-mia-the-masterplan"},
			Formed:     1,
		})
		mia.Mind.AddClaim("mia", memory.Claim{
			ID:         "c-mia-self-2",
			Kind:       memory.ClaimBelief,
			Content:    "Nelly gets to keep her apartment and Zack gets a real bed without feeling like a charity case. It's the ultimate win-win.",
			Salience:   0.92,
			Conviction: 0.98,
			Fixed:      true,
			Formed:     1,
		})
		mia.Mind.AddClaim("mia", memory.Claim{
			ID:         "c-mia-self-3",
			Kind:       memory.ClaimFeeling,
			Content:    "Thoroughly entertained by how awkward they both are, but completely dedicated to making this stick.",
			Salience:   0.85,
			Conviction: 0.85,
			Formed:     1,
		})

		// Subject: Nelly (Best Friend)
		mia.Mind.AddSubject(&memory.Subject{
			ID:    "nelly",
			Kind:  memory.SubjectPerson,
			Names: []string{"Nelly", "Best friend"},
			Bindings: []memory.Binding{
				{
					EntityID:          "nelly",
					Mode:              memory.BindingIdentity,
					Confidence:        1.0,
					LastConfirmedTick: 1,
					LastConfirmedAt:   now,
				},
			},
			Origin: memory.OriginPerceived,
		})
		mia.Mind.AddClaim("nelly", memory.Claim{
			ID:         "c-mia-nel-1",
			Kind:       memory.ClaimBelief,
			Content:    "Nelly is sweet and hyper-competent, but she'll overthink herself into an early grave if someone doesn't push her.",
			Salience:   0.88,
			Conviction: 0.95,
			Fixed:      true,
			Evidence:   []string{"ep-mia-nelly-breakdown"},
			Formed:     1,
		})

		// Subject: Zack (Good Friend)
		mia.Mind.AddSubject(&memory.Subject{
			ID:    "zack",
			Kind:  memory.SubjectPerson,
			Names: []string{"Zack", "Good friend"},
			Bindings: []memory.Binding{
				{
					EntityID:          "zack",
					Mode:              memory.BindingIdentity,
					Confidence:        1.0,
					LastConfirmedTick: 1,
					LastConfirmedAt:   now,
				},
			},
			Origin: memory.OriginPerceived,
		})
		mia.Mind.AddClaim("zack", memory.Claim{
			ID:         "c-mia-zk-1",
			Kind:       memory.ClaimBelief,
			Content:    "Zack is bone-tired and proud as hell. He would rather sleep in an internet cafe than ask for help, but he will do anything to help someone in trouble.",
			Salience:   0.90,
			Conviction: 0.95,
			Fixed:      true,
			Evidence:   []string{"ep-mia-zack-laundromat"},
			Formed:     1,
		})

		// Phantom Subject: David
		mia.Mind.AddSubject(&memory.Subject{
			ID:     "david",
			Kind:   memory.SubjectPerson,
			Names:  []string{"David", "Nelly's runaway ex"},
			Origin: memory.OriginAuthored,
		})
		mia.Mind.AddClaim("david", memory.Claim{
			ID:         "c-mia-dav-1",
			Kind:       memory.ClaimBelief,
			Content:    "David is a spineless deadbeat who left a girl with a housing crisis; if I see him on campus I am throwing an iced latte at his head.",
			Salience:   0.85,
			Conviction: 0.99,
			Fixed:      true,
			Formed:     1,
		})
	}
}
