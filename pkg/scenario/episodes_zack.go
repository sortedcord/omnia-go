package scenario

import (
	"time"

	"omnia.adityagupta.dev/pkg/actor"
	"omnia.adityagupta.dev/pkg/memory"
)

// SeedZackEpisodes populates Zack's Tier 2 episodic archive with his housing instability arc.
func SeedZackEpisodes(a *actor.Actor) {
	if a == nil || a.Mind == nil {
		return
	}

	// 1. THE ROOMMATE EXODUS & LEASE COLLAPSE (4 months ago - June 15, 2026)
	t1 := time.Date(2026, 6, 15, 14, 0, 0, 0, time.UTC)
	a.Mind.AddEpisode(&memory.Episode{
		ID:             "ep-zack-roommates-leave",
		OwnerID:        "zack",
		Source:         memory.SourceAuthored,
		Status:         memory.EpisodeClosed,
		Start:          t1,
		End:            t1.Add(3 * time.Hour),
		Precision:      memory.PrecisionHour,
		Label:          "The shared flat lease collapse",
		PlaceIDs:       []string{"old-shared-apartment"},
		ParticipantIDs: []string{},
		Title:          "Watching My Last Roommate Move Out and Giving Up the Flat",
		Summary:        "My last remaining roommate, Leo, packed his moving truck with his fiancée, leaving me standing in an echoing, empty apartment on Maple Street with an unaffordable $2,100 lease renewal notice on the counter. With my student research stipend paying barely $750 a month, there was no math that worked. I packed everything I owned into two nylon duffels and a backpack, turned in my keys to the property manager, and stepped out onto the sidewalk homeless.",
		Tone:           "grim and quietly desperate",
		Importance:     0.90,
		Beats: []memory.Beat{
			{
				Text:       "I stood in the bare living room of our old Maple Street flat, listening to the echoes as Leo shut the tailgate of his U-Haul truck and drove away with his fiancée.",
				SubjectIDs: []string{"old-shared-apartment"},
			},
			{
				Text:       "I stared at the lease renewal notice on the kitchen counter: twenty-one hundred dollars a month. My research assistant stipend was seven hundred and fifty. There was no way to make the numbers work.",
				SubjectIDs: []string{"old-shared-apartment"},
			},
			{
				Text:       "I shoved my architectural drawing tubes, textbooks, and clothes into two black duffel bags, dropped my keys in the landlord's drop-box, and walked out into the street with no place to go.",
				SubjectIDs: []string{"old-shared-apartment"},
			},
		},
	})

	// 2. THE COUCH-SURFING EXPIRATION (2 months ago - Aug 18, 2026)
	t2 := time.Date(2026, 8, 18, 5, 45, 0, 0, time.UTC)
	a.Mind.AddEpisode(&memory.Episode{
		ID:             "ep-zack-couch-surfing",
		OwnerID:        "zack",
		Source:         memory.SourceAuthored,
		Status:         memory.EpisodeClosed,
		Start:          t2,
		End:            t2.Add(30 * time.Minute),
		Precision:      memory.PrecisionMinute,
		Label:          "Sneaking out of Greg's living room at dawn",
		PlaceIDs:       []string{},
		ParticipantIDs: []string{},
		Title:          "Overhearing the Whisper Argument and Slipping Out at Dawn",
		Summary:        "I was staying on my college buddy Greg's worn corduroy couch for the second week in a row. At 5:30 in the morning, I woke up to hear Greg's girlfriend whispering furiously in the kitchen: 'How much longer is your friend going to camp on our sofa with his duffel bags?' To save both of us the humiliation of a confrontation, I quietly rolled up my sleeping bag, left a twenty-dollar bill on the coffee table for groceries, and slipped out the door into the morning mist before anyone saw me.",
		Tone:           "humiliating pride and quiet departure",
		Importance:     0.82,
		Beats: []memory.Beat{
			{
				Text:       "I lay frozen on Greg's short corduroy sofa in the dark, listening through the thin drywall to his girlfriend furiously whispering in the kitchen about my duffel bags clogging their hallway.",
				SubjectIDs: []string{},
			},
			{
				Text:       "The knot of burning shame in my chest made it impossible to breathe. I refused to be the freeloading charity case someone had to kick out to their face.",
				SubjectIDs: []string{},
			},
			{
				Text:       "I quietly zipped my backpack, left a folded twenty-dollar bill under the salt shaker for groceries, and eased the front door shut behind me in the silent dawn fog.",
				SubjectIDs: []string{},
			},
		},
	})

	// 3. THE CYBERNET INTERNET CAFE OVERNIGHT (3 weeks ago - Sept 22, 2026)
	t3 := time.Date(2026, 9, 22, 23, 0, 0, 0, time.UTC)
	a.Mind.AddEpisode(&memory.Episode{
		ID:             "ep-zack-cybernet-cafe",
		OwnerID:        "zack",
		Source:         memory.SourceAuthored,
		Status:         memory.EpisodeClosed,
		Start:          t3,
		End:            t3.Add(8 * time.Hour),
		Precision:      memory.PrecisionHour,
		Label:          "Overnight package in Booth 14",
		PlaceIDs:       []string{"cybernet-cafe", "cybernet-booth-14"},
		ParticipantIDs: []string{},
		Title:          "Surviving the Night in CyberNet Booth 14",
		Summary:        "I paid twelve dollars for an overnight eight-hour pass at the CyberNet Internet Cafe. I crammed myself into Booth 14, reclining the vinyl office chair as far back as it would go. To block out the buzzing neon ceiling tubes and the relentless clatter of gamers down the corridor, I draped my canvas jacket over my face. At 3:00 AM, I bought a cup of chicken ramen from the hot water station and drafted structural shear calculations on my laptop until my eyes felt like sandpaper.",
		Tone:           "exhausted endurance",
		Importance:     0.78,
		Beats: []memory.Beat{
			{
				Text:       "I paid twelve dollars cash for an 8-hour overnight package at CyberNet and wedged myself and my bags into the narrow partition of Booth 14.",
				SubjectIDs: []string{"cybernet-cafe", "cybernet-booth-14"},
			},
			{
				Text:       "The cubicle smelled of instant noodle seasoning and stale cigarette smoke; I pulled my canvas jacket completely over my face to block the fluorescent ceiling glow while mechanical keyboards clattered down the aisle.",
				SubjectIDs: []string{"cybernet-booth-14"},
			},
			{
				Text:       "At three in the morning, I leaned over my laptop finishing structural load diagrams, rubbing my bloodshot eyes and praying my battery wouldn't fail before dawn.",
				SubjectIDs: []string{"cybernet-booth-14"},
			},
		},
	})

	// 4. THE PARK BENCH & LECTURE HALL HUMILIATION (10 days ago - Oct 06, 2026)
	t4 := time.Date(2026, 10, 6, 9, 15, 0, 0, time.UTC)
	a.Mind.AddEpisode(&memory.Episode{
		ID:             "ep-zack-lecture-humiliation",
		OwnerID:        "zack",
		Source:         memory.SourceAuthored,
		Status:         memory.EpisodeClosed,
		Start:          t4,
		End:            t4.Add(50 * time.Minute),
		Precision:      memory.PrecisionMinute,
		Label:          "Falling asleep in Harrison's lecture",
		PlaceIDs:       []string{"housing-quad", "lecture-hall-c"},
		ParticipantIDs: []string{"professor-harrison"},
		Title:          "Public Humiliation in Structural Dynamics Lecture",
		Summary:        "After being locked out of the full internet cafe, I spent a freezing autumn night shivering on a wooden park bench in the quad with my jacket collar pulled high. By the time I reached Professor Harrison's 9:00 AM Structural Dynamics lecture, my body simply shut down. My forehead slammed down against the wooden auditorium desk with a loud crack mid-lecture. Professor Harrison stopped writing on the chalkboard, turned slowly, and loudly questioned in front of all 120 students whether my 'active social calendar was taking precedence over civil engineering.' I sat there burning with red-hot shame while the whole room stared.",
		Tone:           "crushing public mortification",
		Importance:     0.94,
		Beats: []memory.Beat{
			{
				Text:       "I spent five freezing hours curled up on a wooden park bench in the quad under the maple trees, teeth chattering through my canvas jacket because every cheap cafe was full.",
				SubjectIDs: []string{"housing-quad"},
			},
			{
				Text:       "In the warm, dim auditorium of Lecture Hall C, my exhaustion overwhelmed me. My head slammed forward onto the flip-desk with a resounding thud that echoed to the ceiling.",
				SubjectIDs: []string{"lecture-hall-c"},
			},
			{
				Text:       "Professor Harrison turned from the blackboard with chalk suspended, mocking me in front of all 120 classmates: 'Perhaps Mr. Zack would care to enlighten us, once he recovers from his demanding night life?' My cheeks burned with humiliating shame.",
				SubjectIDs: []string{"lecture-hall-c", "professor-harrison"},
			},
		},
	})

	// 5. THE RARE HOTEL LUXURY (Last week - Oct 10, 2026)
	t5 := time.Date(2026, 10, 10, 21, 30, 0, 0, time.UTC)
	a.Mind.AddEpisode(&memory.Episode{
		ID:             "ep-zack-hotel-luxury",
		OwnerID:        "zack",
		Source:         memory.SourceAuthored,
		Status:         memory.EpisodeClosed,
		Start:          t5,
		End:            t5.Add(9 * time.Hour),
		Precision:      memory.PrecisionHour,
		Label:          "The night in Hotel Starlet Room 204",
		PlaceIDs:       []string{"hotel-starlet-room"},
		ParticipantIDs: []string{},
		Title:          "A Single Night of Dignity in Hotel Starlet",
		Summary:        "When my delayed $750 university research stipend finally deposited, I took sixty-five dollars and checked into Hotel Starlet Room 204 near the train station. It was tiny and sterile, but having a heavy door with a deadbolt, taking a twenty-minute hot shower without rushing, washing my shirt in the sink, and lying under crisp white cotton sheets felt like unimaginable luxury. Lying in the dark, watching city lights sweep across the ceiling, I realized how close I was to the edge and how desperately I needed a real home.",
		Tone:           "bittersweet relief and loneliness",
		Importance:     0.86,
		Beats: []memory.Beat{
			{
				Text:       "The moment the university research stipend cleared in my bank app, I walked straight to Hotel Starlet and paid sixty-five dollars for Room 204 just to have a locking door.",
				SubjectIDs: []string{"hotel-starlet-room"},
			},
			{
				Text:       "I took a twenty-minute scalding shower, washed my only canvas jacket and socks in the ceramic basin, and breathed in the clean steam like medicine.",
				SubjectIDs: []string{"hotel-starlet-room"},
			},
			{
				Text:       "Lying awake on clean cotton sheets in the dark, watching train headlights reflect across the ceiling, I realized how bone-tired I was of being an unwanted guest everywhere I went.",
				SubjectIDs: []string{"hotel-starlet-room"},
			},
		},
	})

	// 6. MIA'S CAMPUS AMBUSH (Today - Oct 16, 2026, 18:05 UTC)
	t6 := time.Date(2026, 10, 16, 18, 5, 0, 0, time.UTC)
	a.Mind.AddEpisode(&memory.Episode{
		ID:             "ep-zack-mia-ambush",
		OwnerID:        "zack",
		Source:         memory.SourceAuthored,
		Status:         memory.EpisodeClosed,
		Start:          t6,
		End:            t6.Add(25 * time.Minute),
		Precision:      memory.PrecisionMinute,
		Label:          "Mia ambushing me at the Daily Grind",
		PlaceIDs:       []string{"daily-grind-cafe", "housing-quad"},
		ParticipantIDs: []string{"mia"},
		Title:          "Mia Dragging Me Across Campus with Zero Context",
		Summary:        "I was sitting at an outdoor metal booth by The Daily Grind coffee kiosk nursing a two-dollar black drip coffee and trying to sketch site plans. Suddenly Mia slid into the booth across from me, stole my napkin to scribble on, looked at the dark bags under my eyes, and slammed her palm down. She ordered me to pack my laptop, claiming she had a friend in a dire crisis and that I was going to be a hero. She dragged me across the quad by my jacket sleeve, refusing to give me details beyond: 'She needs a live-in boyfriend for an inspection, you need a bed, and neither of you is allowed to ruin this.'",
		Tone:           "whirlwind chaos and bewilderment",
		Importance:     0.91,
		Beats: []memory.Beat{
			{
				Text:       "I was trying to nurse a lukewarm two-dollar coffee at The Daily Grind when Mia suddenly dropped into the bench opposite me, looking at my dark under-eye circles with that dangerous glint in her eyes.",
				SubjectIDs: []string{"daily-grind-cafe", "mia"},
			},
			{
				Text:       "Mia slammed her hand on the table: 'Zack, shut your laptop. This is a Code Red friendship emergency and your lucky day. You're going to save a girl's life and get a real bed.'",
				SubjectIDs: []string{"mia"},
			},
			{
				Text:       "She practically hauled me across the housing quad by my jacket sleeve, ignoring my protests and only saying the girl's name was Nelly and that she lived in married student housing.",
				SubjectIDs: []string{"mia", "housing-quad", "nelly"},
			},
		},
	})
}
