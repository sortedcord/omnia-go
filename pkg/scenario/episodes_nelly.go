package scenario

import (
	"time"

	"omnia.adityagupta.dev/pkg/actor"
	"omnia.adityagupta.dev/pkg/memory"
)

// SeedNellyEpisodes populates Nelly's Tier 2 episodic archive with rich, immersive backstory.
func SeedNellyEpisodes(a *actor.Actor) {
	if a == nil || a.Mind == nil {
		return
	}

	// 1. THE MOVE-IN PROMISE (8 months ago - Feb 14, 2026)
	t1 := time.Date(2026, 2, 14, 19, 30, 0, 0, time.UTC)
	a.Mind.AddEpisode(&memory.Episode{
		ID:             "ep-nelly-david-movein",
		OwnerID:        "nelly",
		Source:         memory.SourceAuthored,
		Status:         memory.EpisodeClosed,
		Start:          t1,
		End:            t1.Add(3 * time.Hour),
		Precision:      memory.PrecisionHour,
		Label:          "Valentine's Day moving into Apt 3B",
		PlaceIDs:       []string{"living-room"},
		ParticipantIDs: []string{"david"},
		Title:          "Moving into the Subsidized Apartment with David",
		Summary:        "We spent all evening unpacking flat-pack IKEA boxes on the bare floor, laughing until our stomachs hurt because David put the coffee table drawer in upside down. We shared a single pair of wooden chopsticks eating pork ramen out of a paper carton. David held up our co-tenancy agreement like a trophy and swore we had beaten the system and would never need our parents' help again.",
		Tone:           "warm and hopeful",
		Importance:     0.90,
		Beats: []memory.Beat{
			{
				Text:       "David and I sat cross-legged on the bare wooden floor of 3B, surrounded by flat-pack cardboard and styrofoam dust, laughing because he'd jammed the coffee table drawer in backwards.",
				SubjectIDs: []string{"david", "living-room", "coffee-table"},
			},
			{
				Text:       "We split a steaming carton of pork bone ramen using a single pair of chopsticks because we couldn't find the cutlery box, taking turns and burning our tongues.",
				SubjectIDs: []string{"david", "living-room"},
			},
			{
				Text:       "David held up the signed Housing Authority lease and said, 'Three hundred bucks a month in married student housing, Nel. We beat the system. We're never asking our families for a dime again.'",
				SubjectIDs: []string{"david", "housing-authority"},
			},
		},
	})

	// 2. THE SUNDAY MARKET & LITTLE QUIRKS (5 months ago - May 10, 2026)
	t2 := time.Date(2026, 5, 10, 11, 15, 0, 0, time.UTC)
	a.Mind.AddEpisode(&memory.Episode{
		ID:             "ep-nelly-david-market",
		OwnerID:        "nelly",
		Source:         memory.SourceAuthored,
		Status:         memory.EpisodeClosed,
		Start:          t2,
		End:            t2.Add(2 * time.Hour),
		Precision:      memory.PrecisionHour,
		Label:          "Sunday farmers market with David",
		PlaceIDs:       []string{"balcony", "housing-quad"},
		ParticipantIDs: []string{"david"},
		Title:          "Buying the Balcony Lavender Plant with David",
		Summary:        "David dragged me through the Sunday farmers market, buying me a potted lavender for our balcony even though I warned him I kill succulents. He teased me about my color-coded revision notes while we ate honey pastries. On the walk back, he held my hand and promised we'd paint the living room wall pale sage green over summer break.",
		Tone:           "tender and playful",
		Importance:     0.75,
		Beats: []memory.Beat{
			{
				Text:       "David insisted on buying a fragrant potted lavender plant for our small balcony, laughing when I told him my botanical track record was an ecological disaster.",
				SubjectIDs: []string{"david", "balcony", "lavender-pot"},
			},
			{
				Text:       "He teased me about my highlighters while brushing powdered sugar off my chin, promising we'd paint the living room wall sage green as soon as midterms wrapped.",
				SubjectIDs: []string{"david", "living-room"},
			},
		},
	})

	// 3. THE COLD SHIFT & THE SILENT DINNER (2 months ago - Aug 24, 2026)
	t3 := time.Date(2026, 8, 24, 20, 10, 0, 0, time.UTC)
	a.Mind.AddEpisode(&memory.Episode{
		ID:             "ep-nelly-david-distance",
		OwnerID:        "nelly",
		Source:         memory.SourceAuthored,
		Status:         memory.EpisodeClosed,
		Start:          t3,
		End:            t3.Add(90 * time.Minute),
		Precision:      memory.PrecisionMinute,
		Label:          "The ruined curry dinner",
		PlaceIDs:       []string{"kitchenette"},
		ParticipantIDs: []string{"david"},
		Title:          "The Silent Dinner and David's Snapping",
		Summary:        "I spent two hours cooking chicken curry from scratch to celebrate David's project milestone. He arrived two hours late without texting, smelling of cheap vape smoke and cold drizzle. He ate in suffocating silence with his phone vibrating face-down on his knee. When I gently asked about signing the lease renewal form on the fridge, he slammed his spoon down and barked: 'Stop policing me, Nelly. Can I eat one meal without an administrative audit?'",
		Tone:           "strained and chilling",
		Importance:     0.85,
		Beats: []memory.Beat{
			{
				Text:       "I kept the chicken curry warm on the stove for two hours, watching the clock tick past eight while David ignored my three messages.",
				SubjectIDs: []string{"kitchenette", "david"},
			},
			{
				Text:       "When David finally stumbled in, he barely mumbled an apology, smelling of vape smoke and staring at his buzzing phone tucked against his thigh.",
				SubjectIDs: []string{"david", "kitchenette"},
			},
			{
				Text:       "When I pointed out the Housing Authority renewal form taped to the fridge, he slammed his spoon down and shouted, 'Stop treating me like an administrative case file, Nelly!' and walked out to sleep on the sofa.",
				SubjectIDs: []string{"david", "kitchenette", "housing-authority"},
			},
		},
	})

	// 4. THE ABANDONMENT (6 weeks ago - Sept 04, 2026)
	t4 := time.Date(2026, 9, 4, 16, 45, 0, 0, time.UTC)
	a.Mind.AddEpisode(&memory.Episode{
		ID:             "ep-nelly-david-abandonment",
		OwnerID:        "nelly",
		Source:         memory.SourceAuthored,
		Status:         memory.EpisodeClosed,
		Start:          t4,
		End:            t4.Add(45 * time.Minute),
		Precision:      memory.PrecisionMinute,
		Label:          "The day David walked out",
		PlaceIDs:       []string{"living-room", "hallway"},
		ParticipantIDs: []string{"david"},
		Title:          "Coming Home to David's Packed Bags and Desertion",
		Summary:        "I came home from my organic chemistry lab to find two packed black duffels in the hallway and David's wardrobe hangers clattering empty. He wouldn't look me in the eye as he zipped his windbreaker, muttering that he needed space and was moving into an off-campus house with buddies. When I cried that the lease required two full-time students, he told me not to be dramatic, promised he'd Venmo his half of the rent, and got into an Uber. He never sent a dollar and blocked my number three days later.",
		Tone:           "devastating betrayal",
		Importance:     0.98,
		Beats: []memory.Beat{
			{
				Text:       "I walked through the front door and froze: David's heavy black duffel bags were lined up in the hallway, and the closet doors hung open with empty wire hangers.",
				SubjectIDs: []string{"living-room", "hallway", "david"},
			},
			{
				Text:       "David kept his back to me while zipping his jacket, muttering that he couldn't breathe in this relationship and had found an open room with guys from his fraternity.",
				SubjectIDs: []string{"david"},
			},
			{
				Text:       "I grabbed his sleeve begging him to remember the co-tenancy rules, but he shook me off: 'Don't be dramatic, Nel. I'll Venmo my half. Just don't tell housing anything.' Then he stepped into an Uber and vanished.",
				SubjectIDs: []string{"david", "housing-authority", "living-room"},
			},
		},
	})

	// 5. THE OFFICIAL AUDIT SUMMONS (4 days ago - Oct 12, 2026)
	t5 := time.Date(2026, 10, 12, 17, 20, 0, 0, time.UTC)
	a.Mind.AddEpisode(&memory.Episode{
		ID:             "ep-nelly-audit-letter",
		OwnerID:        "nelly",
		Source:         memory.SourceAuthored,
		Status:         memory.EpisodeClosed,
		Start:          t5,
		End:            t5.Add(30 * time.Minute),
		Precision:      memory.PrecisionMinute,
		Label:          "The official housing board notice",
		PlaceIDs:       []string{"mailroom-lobby"},
		ParticipantIDs: []string{},
		Title:          "Finding the Mandatory Co-Tenancy Inspection Notice",
		Summary:        "I checked mailbox 3B in the ground-floor lobby and pulled out the thick manila envelope stamped with the University Housing Compliance seal. The letter announced an unannounced in-person co-tenancy verification inspection scheduled for Thursday October 22 at 18:30. The floor seemed to drop away under my sneakers: if David isn't living here, the lease is immediately revoked, I owe $6,800 in back-rent, and my parents will force me to drop out and move back to their basement.",
		Tone:           "paralyzing dread",
		Importance:     0.95,
		Beats: []memory.Beat{
			{
				Text:       "My hands started shaking in the dim mailroom as I tore open the manila envelope stamped with the University Housing seal.",
				SubjectIDs: []string{"mailroom-lobby", "housing-authority"},
			},
			{
				Text:       "The bold black text read: 'MANDATORY RANDOM CO-TENANCY VERIFICATION: Apt 3B, Thursday Oct 22 at 18:30. Both registered co-tenants must be physically present.'",
				SubjectIDs: []string{"housing-authority"},
			},
			{
				Text:       "I realized with total sickening clarity that without David, I was facing immediate eviction, six thousand dollars in market back-rent, and moving back into my parents' basement in utter disgrace.",
				SubjectIDs: []string{"housing-authority", "nellys-parents"},
			},
		},
	})

	// 6. THE MIDNIGHT BREAKDOWN & MIA'S SCHEME (Yesterday - Oct 15, 2026)
	t6 := time.Date(2026, 10, 15, 23, 10, 0, 0, time.UTC)
	a.Mind.AddEpisode(&memory.Episode{
		ID:             "ep-nelly-mia-scheme",
		OwnerID:        "nelly",
		Source:         memory.SourceAuthored,
		Status:         memory.EpisodeClosed,
		Start:          t6,
		End:            t6.Add(2 * time.Hour),
		Precision:      memory.PrecisionMinute,
		Label:          "Mia pitching the fake boyfriend plan",
		PlaceIDs:       []string{"kitchenette", "living-room"},
		ParticipantIDs: []string{"mia"},
		Title:          "Mia Finding Me on the Kitchen Floor and Pitching Zack",
		Summary:        "Mia used her spare key to enter Apt 3B and found me huddled against the lower kitchen cabinets hyperventilating over crumpled appeal drafts. Instead of pitying me, Mia poured me a glass of tap water, read the inspection notice, and slammed her hand on the counter. She told me: 'You are not losing this lease, and you are not moving back with your parents. We don't need David. We just need a guy who doesn't look like a criminal and can sit on a couch looking like your boyfriend for twenty minutes. I know the exact person, and trust me, he needs this as much as you do.'",
		Tone:           "hysterical panic turning into frantic conspiracy",
		Importance:     0.92,
		Beats: []memory.Beat{
			{
				Text:       "Mia found me sitting on the cold kitchen linoleum hugging my knees, hyperventilating amidst half-written appeal letters to the housing office.",
				SubjectIDs: []string{"kitchenette", "mia", "housing-authority"},
			},
			{
				Text:       "Mia yanked the audit letter from my fingers, scanned it in ten seconds, and scoffed: 'You are not paying six grand to a university with a three-billion-dollar endowment, Nel.'",
				SubjectIDs: []string{"mia", "housing-authority"},
			},
			{
				Text:       "She paced across the kitchen: 'We just need a stand-in boyfriend. Someone dependable, someone who won't be weird. I know a guy named Zack. He's an architecture student, he's decent, and he desperately needs a break. Tomorrow, I'm bringing him over.'",
				SubjectIDs: []string{"mia", "zack", "kitchenette"},
			},
		},
	})
}
