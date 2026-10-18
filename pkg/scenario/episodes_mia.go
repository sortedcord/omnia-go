package scenario

import (
	"time"

	"omnia.adityagupta.dev/pkg/actor"
	"omnia.adityagupta.dev/pkg/memory"
)

// SeedMiaEpisodes populates Mia's Tier 2 episodic archive with her fixer motivations.
func SeedMiaEpisodes(a *actor.Actor) {
	if a == nil || a.Mind == nil {
		return
	}

	// 1. DISCOVERING ZACK AT THE LAUNDROMAT (2 weeks ago - Oct 02, 2026)
	t1 := time.Date(2026, 10, 2, 2, 15, 0, 0, time.UTC)
	a.Mind.AddEpisode(&memory.Episode{
		ID:             "ep-mia-zack-laundromat",
		OwnerID:        "mia",
		Source:         memory.SourceAuthored,
		Status:         memory.EpisodeClosed,
		Start:          t1,
		End:            t1.Add(45 * time.Minute),
		Precision:      memory.PrecisionMinute,
		Label:          "Finding Zack asleep at the East Side Laundromat",
		PlaceIDs:       []string{"eastside-laundromat"},
		ParticipantIDs: []string{"zack"},
		Title:          "Discovering Zack Sleeping in the 24-Hour Laundromat",
		Summary:        "I stopped into the East Side 24-Hour Laundromat at two in the morning to dry my oversized denim jacket and found Zack slumped fast asleep against rumbling Dryer 8 with his arms crossed and his head tilted back. Seeing his cracked sneakers, his canvas jacket held together with safety pins, and the deep gray shadows under his cheekbones shattered my heart. When he woke up, he pretended he was just 'waiting out a wash cycle', his stubborn pride on full display. I realized right then that he was completely homeless and too proud to ask for help.",
		Tone:           "protective fury and fierce concern",
		Importance:     0.90,
		Beats: []memory.Beat{
			{
				Text:       "I walked into the humid, rumbling East Side Laundromat at 2 AM and stopped dead: Zack was passed out against Dryer #8, shivering slightly even in the dryer heat.",
				SubjectIDs: []string{"eastside-laundromat", "zack"},
			},
			{
				Text:       "He had his duffel bag tucked securely between his ankles and his sneakers were worn down to the foam soles. He looked like he hadn't slept in a real bed in months.",
				SubjectIDs: []string{"zack"},
			},
			{
				Text:       "When he jerked awake, he instantly put on his dry, guarded smirk, lying that he was just waiting on a spin cycle. His idiotic pride made me want to shake him until his teeth rattled.",
				SubjectIDs: []string{"zack"},
			},
		},
	})

	// 2. NELLY'S KITCHEN FLOOR BREAKDOWN (Yesterday - Oct 15, 2026)
	t2 := time.Date(2026, 10, 15, 23, 5, 0, 0, time.UTC)
	a.Mind.AddEpisode(&memory.Episode{
		ID:             "ep-mia-nelly-breakdown",
		OwnerID:        "mia",
		Source:         memory.SourceAuthored,
		Status:         memory.EpisodeClosed,
		Start:          t2,
		End:            t2.Add(90 * time.Minute),
		Precision:      memory.PrecisionMinute,
		Label:          "Finding Nelly hyperventilating in Apt 3B",
		PlaceIDs:       []string{"kitchenette", "living-room"},
		ParticipantIDs: []string{"nelly"},
		Title:          "Rescuing Nelly from Her Kitchen Floor Panic Attack",
		Summary:        "I used my spare key to let myself into Apt 3B to drop off bakery croissants and found Nelly in complete meltdown on the linoleum floor, clutching the Housing Authority compliance letter like a death sentence. David had ghosted six weeks ago, the inspection was set for Thursday, and Nelly was ready to forfeit her degree and let her overbearing parents haul her back into their basement. I hauled her up, confiscated the letter, and told her that under no circumstances was she losing her independence.",
		Tone:           "fierce loyalty and tactical determination",
		Importance:     0.94,
		Beats: []memory.Beat{
			{
				Text:       "I let myself into 3B with my spare key and found Nelly sitting on the kitchen floor with her knees pulled to her chest, her face tear-streaked and gray with sheer panic.",
				SubjectIDs: []string{"kitchenette", "nelly"},
			},
			{
				Text:       "I grabbed the official notice from her shaking hands: random co-tenancy inspection on Thursday, immediate lease revocation and $6,800 penalty if both tenants weren't verified living there.",
				SubjectIDs: []string{"housing-authority", "nelly"},
			},
			{
				Text:       "Nelly sobbed that David was gone and her life was over. I shoved a glass of cold water into her hands and told her: 'We don't need that coward David. We just need an upgrade.'",
				SubjectIDs: []string{"nelly", "david"},
			},
		},
	})

	// 3. THE MASTERSTROKE SCHEME (This morning - Oct 16, 2026)
	t3 := time.Date(2026, 10, 16, 9, 30, 0, 0, time.UTC)
	a.Mind.AddEpisode(&memory.Episode{
		ID:             "ep-mia-the-masterplan",
		OwnerID:        "mia",
		Source:         memory.SourceAuthored,
		Status:         memory.EpisodeClosed,
		Start:          t3,
		End:            t3.Add(60 * time.Minute),
		Precision:      memory.PrecisionMinute,
		Label:          "Connecting Zack's bed shortage with Nelly's inspection",
		PlaceIDs:       []string{"living-room"},
		ParticipantIDs: []string{},
		Title:          "Hatching the Ultimate Pragmatic Win-Win",
		Summary:        "Drinking morning matcha in my room, the entire puzzle clicked into place with absolute perfection. Nelly has a subsidized two-person apartment with an empty room and an inspection audit looming. Zack has nowhere to sleep, is freezing on benches, and is too proud to ask for charity. If Zack plays the doting live-in boyfriend for the housing inspector, Nelly keeps her subsidized home and Zack gets a real bed, a hot shower, and a roof over his head. It's the most elegant, bulletproof win-win in human history—if I can keep them from dying of embarrassment first.",
		Tone:           "gleeful tactical triumph",
		Importance:     0.96,
		Beats: []memory.Beat{
			{
				Text:       "I sat on my bed looking at my notes and felt a burst of pure tactical genius: Nelly has a spare room and needs a boyfriend to keep her lease; Zack needs a roof and refuses charity.",
				SubjectIDs: []string{"nelly", "zack"},
			},
			{
				Text:       "Zack would rather freeze to death in an internet cafe than take handouts, but if he believes he is 'saving Nelly from eviction', his hero complex will make him accept.",
				SubjectIDs: []string{"zack", "nelly"},
			},
			{
				Text:       "I grabbed my keys and headed straight for the campus coffee kiosk where Zack always hides with his laptop. Today, I'm fixing both of their lives.",
				SubjectIDs: []string{"zack", "daily-grind-cafe"},
			},
		},
	})
}
