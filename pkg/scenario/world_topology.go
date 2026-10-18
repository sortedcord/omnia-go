package scenario

import (
	"omnia.adityagupta.dev/pkg/sim"
)

// WorldLocations holds references to key spawned spaces and landmark entities.
type WorldLocations struct {
	LivingRoom     *sim.Entity
	Kitchenette    *sim.Entity
	Balcony        *sim.Entity
	Hallway        *sim.Entity
	GuestBathroom  *sim.Entity
	NellyBedroom   *sim.Entity
	MasterBathroom *sim.Entity
	Breezeway3F    *sim.Entity
	Stairwell3F    *sim.Entity
	MailroomLobby  *sim.Entity
	HousingQuad    *sim.Entity
	DailyGrindCafe *sim.Entity
	ArchStudio     *sim.Entity
	LectureHallC   *sim.Entity
	LibraryReading *sim.Entity
	LibraryStacks  *sim.Entity
	CyberNetCafe   *sim.Entity
	CyberNetBooth  *sim.Entity
	Laundromat     *sim.Entity
	HotelStarlet   *sim.Entity
	SevenStarMart  *sim.Entity
	MetroUnderpass *sim.Entity

	// Key landmark props
	CoffeeTable       *sim.Entity
	RelationshipNotes *sim.Entity
	StagedFlannel     *sim.Entity
	StagedToothbrush  *sim.Entity
	PhotoshopPhoto    *sim.Entity
	AuditLetter       *sim.Entity
}

// BuildWorldTopology constructs the 22 interconnected spaces and their physical props.
func BuildWorldTopology(world *sim.Simulation) (*WorldLocations, error) {
	locs := &WorldLocations{}

	// =========================================================================
	// 1. NELLY'S APARTMENT (BRYN HALL, APARTMENT 3B)
	// =========================================================================

	// Living Room (Center stage of opening scene)
	livingRoom := world.SpawnSpace(
		"living-room",
		"Nelly's Apartment - Living Room",
		"A warm, slightly cluttered college apartment living room. Everything has been hastily staged to look like a loving couple lives here.",
		true,
	)
	_ = livingRoom.Attributes.Set("Atmosphere", "Warm, faintly smelling of lavender, coffee, and printed paper")
	_ = livingRoom.Attributes.Set("Social Tension", "Thick, awkward relational panic")
	locs.LivingRoom = livingRoom

	// Open Kitchenette
	kitchenette := world.SpawnSpace(
		"kitchenette",
		"Open Kitchenette",
		"A compact kitchen with laminate countertops and a sink, separated from the living room by an open breakfast counter.",
		true,
	)
	_ = kitchenette.Attributes.Set("Cleanliness", "Hastily wiped down; lingering smell of instant noodles and cold coffee")
	locs.Kitchenette = kitchenette

	// Portals: Living Room <-> Kitchenette
	_, _ = world.SpawnPortal("counter-pass", "Kitchen Breakfast Counter", livingRoom.ID, kitchenette.ID, "kitchen counter")
	_, _ = world.SpawnPortal("counter-pass-return", "Breakfast Counter to Living Room", kitchenette.ID, livingRoom.ID, "into living room")

	// Small Balcony
	balcony := world.SpawnSpace(
		"balcony",
		"Small Balcony",
		"A narrow concrete balcony with a black metal railing overlooking the inner housing quad.",
		true,
	)
	_ = balcony.Attributes.Set("Weather", "Chilly October evening air, faint breeze carrying campus noises")
	locs.Balcony = balcony

	// Portals: Living Room <-> Balcony
	_, _ = world.SpawnPortal("balcony-door", "Sliding Glass Balcony Door", livingRoom.ID, balcony.ID, "out onto the balcony")
	_, _ = world.SpawnPortal("balcony-door-return", "Balcony to Living Room", balcony.ID, livingRoom.ID, "back inside the warm living room")

	// Apartment Hallway
	hallway := world.SpawnSpace(
		"hallway",
		"Apartment Hallway",
		"A short linoleum hallway connecting the living room to the private bedrooms and bathrooms.",
		true,
	)
	locs.Hallway = hallway

	// Portals: Living Room <-> Hallway
	_, _ = world.SpawnPortal("living-to-hall", "Hallway Arch", livingRoom.ID, hallway.ID, "into the private hallway")
	_, _ = world.SpawnPortal("hall-to-living", "Living Room Entry", hallway.ID, livingRoom.ID, "into the living room")

	// Guest Bathroom (Off hallway)
	guestBath := world.SpawnSpace(
		"guest-bathroom",
		"Hallway Guest Bathroom",
		"A small half-bath with a pedestal sink, white tile, and a fluorescent mirror light.",
		true,
	)
	locs.GuestBathroom = guestBath

	// Portals: Hallway <-> Guest Bathroom
	_, _ = world.SpawnPortal("hall-to-guest-bath", "Guest Bathroom Door", hallway.ID, guestBath.ID, "into guest bathroom")
	_, _ = world.SpawnPortal("guest-bath-to-hall", "Bathroom Exit", guestBath.ID, hallway.ID, "back into hallway")

	// Nelly's Bedroom
	nellyBedroom := world.SpawnSpace(
		"nelly-bedroom",
		"Nelly's Bedroom",
		"A tidy, cozy bedroom with a twin-XL bed, a heavily organized study desk, and a half-empty closet.",
		true,
	)
	_ = nellyBedroom.Attributes.Set("Atmosphere", "Intense focus; study notes pinned on corkboard")
	locs.NellyBedroom = nellyBedroom

	// Portals: Hallway <-> Nelly's Bedroom
	_, _ = world.SpawnPortal("hall-to-bedroom", "Bedroom Door", hallway.ID, nellyBedroom.ID, "into Nelly's bedroom")
	_, _ = world.SpawnPortal("bedroom-to-hall", "Bedroom Exit", nellyBedroom.ID, hallway.ID, "into hallway")

	// Master En-Suite Bathroom
	masterBath := world.SpawnSpace(
		"master-bathroom",
		"En-Suite Master Bathroom",
		"A full bathroom attached to Nelly's bedroom with a shower-tub combo and laundry hamper.",
		true,
	)
	locs.MasterBathroom = masterBath

	// Portals: Nelly's Bedroom <-> Master Bathroom
	_, _ = world.SpawnPortal("bedroom-to-master-bath", "En-Suite Bathroom Door", nellyBedroom.ID, masterBath.ID, "into master bathroom")
	_, _ = world.SpawnPortal("master-bath-to-bedroom", "Master Bathroom Exit", masterBath.ID, nellyBedroom.ID, "back into bedroom")

	// =========================================================================
	// 2. BRYN HALL PUBLIC SPACES (BUILDING 3)
	// =========================================================================

	// 3rd Floor Breezeway (Outside Apartment 3B)
	breezeway := world.SpawnSpace(
		"breezeway-3f",
		"Bryn Hall 3rd Floor - Outside Apartment 3B",
		"An open-air concrete breezeway walkway on the third floor of Bryn Hall, right outside the door of Apartment 3B. The metal railing overlooks the evening campus quad below.",
		true,
	)
	_ = breezeway.Attributes.Set("Atmosphere", "Cool autumn evening air, quiet residential corridor")
	locs.Breezeway3F = breezeway

	// Front Door to Apartment 3B (Portal between Breezeway and Living Room)
	aptDoor, _ := world.SpawnPortal("breezeway-to-apt", "Apartment 3B Front Door", breezeway.ID, livingRoom.ID, "into Apartment 3B")
	aptDoor.Description = "A painted wooden door with bronze numerals '3B'. It is closed but unlocked."
	aptDoor.Portal.Closed = true
	aptDoor.Portal.Locked = false

	aptDoorReturn, _ := world.SpawnPortal("apt-front-door", "Front Door to Breezeway", livingRoom.ID, breezeway.ID, "out onto the 3rd floor breezeway")
	aptDoorReturn.Description = "The front door leading out to the 3rd floor breezeway."
	aptDoorReturn.Portal.Closed = true
	aptDoorReturn.Portal.Locked = false

	_, _ = world.SpawnEntity(
		"doorbell-3b",
		"Apartment 3B Doorbell",
		"A small rectangular white buzzer beside the apartment door.",
		breezeway.ID,
		sim.Link{Relation: sim.RelOn},
	)

	// Stairwell Landing 3F
	stairwell := world.SpawnSpace(
		"stairwell-3f",
		"3rd Floor Stairwell & Elevator Landing",
		"A painted cinderblock stairwell echoing with distant footsteps and the hum of an old hydraulic elevator.",
		true,
	)
	locs.Stairwell3F = stairwell

	// Portals: Breezeway <-> Stairwell
	_, _ = world.SpawnPortal("breezeway-to-stairs", "Heavy Fire Door to Stairwell", breezeway.ID, stairwell.ID, "into the stairwell landing")
	_, _ = world.SpawnPortal("stairs-to-breezeway", "Fire Door to 3rd Floor", stairwell.ID, breezeway.ID, "onto the 3rd floor breezeway")

	// Ground Floor Mailroom & Lobby
	mailroom := world.SpawnSpace(
		"mailroom-lobby",
		"Ground Floor Mailroom & Lobby",
		"A fluorescent-lit lobby lined with rows of dented brass mailboxes and a bulletin board covered in administrative notices.",
		true,
	)
	locs.MailroomLobby = mailroom

	// Portals: Stairwell <-> Mailroom Lobby
	_, _ = world.SpawnPortal("stairs-to-ground", "Stairwell Down to Lobby", stairwell.ID, mailroom.ID, "downstairs to ground lobby")
	_, _ = world.SpawnPortal("ground-to-stairs", "Stairwell Upstairs", mailroom.ID, stairwell.ID, "upstairs toward residential floors")

	// Housing Quad / Courtyard
	housingQuad := world.SpawnSpace(
		"housing-quad",
		"Bryn Hall Courtyard & Quad",
		"A grassy square between university housing blocks with paved walkways, mature maple trees, and green-painted park benches.",
		true,
	)
	_ = housingQuad.Attributes.Set("Lighting", "Dusk falling, amber campus lampposts flickering to life")
	locs.HousingQuad = housingQuad

	// Portals: Mailroom <-> Housing Quad
	_, _ = world.SpawnPortal("lobby-to-quad", "Glass Double Doors to Quad", mailroom.ID, housingQuad.ID, "outside into the courtyard quad")
	_, _ = world.SpawnPortal("quad-to-lobby", "Bryn Hall Main Entrance", housingQuad.ID, mailroom.ID, "inside Bryn Hall lobby")

	// =========================================================================
	// 3. CAMPUS CORE & ACADEMICS
	// =========================================================================

	// The Daily Grind Coffee Kiosk
	dailyGrind := world.SpawnSpace(
		"daily-grind-cafe",
		"The Daily Grind Coffee Kiosk",
		"A busy outdoor covered coffee kiosk smelling of roasted espresso beans, cinnamon syrup, and steamed oat milk.",
		true,
	)
	locs.DailyGrindCafe = dailyGrind

	// Portals: Quad <-> Daily Grind Cafe
	_, _ = world.SpawnPortal("quad-to-cafe", "Campus Pathway to Coffee Kiosk", housingQuad.ID, dailyGrind.ID, "over to the coffee kiosk")
	_, _ = world.SpawnPortal("cafe-to-quad", "Pathway to Bryn Hall Quad", dailyGrind.ID, housingQuad.ID, "back toward the housing quad")

	// Architecture & Design Studio 4B
	archStudio := world.SpawnSpace(
		"architecture-studio",
		"Architecture & Design Studio 4B",
		"A high-ceilinged loft studio with tall industrial windows, rows of wooden drafting tables, and scattered cardboard models.",
		true,
	)
	_ = archStudio.Attributes.Set("Smell", "Pencil shavings, wood glue, and dry-erase marker")
	locs.ArchStudio = archStudio

	// Portals: Daily Grind <-> Architecture Studio
	_, _ = world.SpawnPortal("cafe-to-studio", "North Hall Entrance", dailyGrind.ID, archStudio.ID, "into the Architecture building")
	_, _ = world.SpawnPortal("studio-to-cafe", "Studio Exit", archStudio.ID, dailyGrind.ID, "outside toward the coffee kiosk")

	// Lecture Hall C
	lectureHallC := world.SpawnSpace(
		"lecture-hall-c",
		"Engineering Hall - Lecture Hall C",
		"A steep, tiered 150-seat lecture auditorium with flip-down wooden desks, a large chalkboard, and harsh overhead spotlights.",
		true,
	)
	locs.LectureHallC = lectureHallC

	// Portals: Arch Studio <-> Lecture Hall C
	_, _ = world.SpawnPortal("studio-to-lecture", "Cross-Campus Walkway", archStudio.ID, lectureHallC.ID, "over to Engineering Hall")
	_, _ = world.SpawnPortal("lecture-to-studio", "Engineering Hall Exit", lectureHallC.ID, archStudio.ID, "toward Architecture building")

	// Central Library - 24hr Reading Room
	libraryReading := world.SpawnSpace(
		"library-reading-room",
		"Central Library - 24hr Reading Room",
		"A solemn, cavernous quiet hall with heavy dark oak tables, brass reading lamps with green glass shades, and tired students huddled over textbooks.",
		true,
	)
	_ = libraryReading.Attributes.Set("Acoustics", "Dead silence; faint rustle of turning pages and computer mouse clicks")
	locs.LibraryReading = libraryReading

	// Portals: Quad <-> Library Reading Room
	_, _ = world.SpawnPortal("quad-to-library", "Stone Steps to Central Library", housingQuad.ID, libraryReading.ID, "inside the central library")
	_, _ = world.SpawnPortal("library-to-quad", "Library Exit", libraryReading.ID, housingQuad.ID, "out onto the campus quad")

	// Library 3rd Floor Book Stacks
	libraryStacks := world.SpawnSpace(
		"library-stacks-3f",
		"Library 3rd Floor Book Stacks",
		"Towering metal aisles of engineering and design monographs, with quiet window alcoves overlooking the campus lights.",
		true,
	)
	locs.LibraryStacks = libraryStacks

	// Portals: Reading Room <-> Book Stacks
	_, _ = world.SpawnPortal("reading-to-stacks", "Spiral Iron Staircase Up", libraryReading.ID, libraryStacks.ID, "up to the 3rd floor book stacks")
	_, _ = world.SpawnPortal("stacks-to-reading", "Spiral Staircase Down", libraryStacks.ID, libraryReading.ID, "down to the 24hr reading room")

	// =========================================================================
	// 4. OFF-CAMPUS URBAN SPOTS (ZACK'S SURVIVAL MAP)
	// =========================================================================

	// 24-Hour "CyberNet" Internet Cafe
	cyberNet := world.SpawnSpace(
		"cybernet-cafe",
		"24-Hour 'CyberNet' Internet Cafe",
		"A subterranean sanctuary lit by neon blue tubes, smelling of instant ramen and ozone, filled with rows of enclosed gaming pods.",
		true,
	)
	locs.CyberNetCafe = cyberNet

	// CyberNet - Booth 14 (Zack's frequent shelter)
	booth14 := world.SpawnSpace(
		"cybernet-booth-14",
		"CyberNet - Booth 14",
		"A narrow vinyl cubicle with a reclining faux-leather armchair, a PC tower with glowing fans, and a headphone rack.",
		true,
	)
	_ = booth14.Attributes.Set("Comfort", "Cramped but private; charging outlet working")
	locs.CyberNetBooth = booth14

	// Portals: CyberNet <-> Booth 14
	_, _ = world.SpawnPortal("cyber-to-booth", "Sliding Partition to Booth 14", cyberNet.ID, booth14.ID, "into Booth 14")
	_, _ = world.SpawnPortal("booth-to-cyber", "Booth Exit", booth14.ID, cyberNet.ID, "out to common internet cafe hall")

	// East Side 24-Hour Laundromat
	laundromat := world.SpawnSpace(
		"eastside-laundromat",
		"East Side 24-Hour Laundromat",
		"A warm, cavernous room vibrating with forty rumbling tumble-dryers, plastic bucket chairs, and a coin changer on the wall.",
		true,
	)
	locs.Laundromat = laundromat

	// Hotel Starlet - Room 204
	hotelRoom := world.SpawnSpace(
		"hotel-starlet-room",
		"Hotel Starlet - Room 204",
		"A shoebox-sized budget business hotel room with a single spring mattress, clean white cotton sheets, and a tiny plastic shower module.",
		true,
	)
	_ = hotelRoom.Attributes.Set("Luxury Level", "Modest budget lodging; quiet and private")
	locs.HotelStarlet = hotelRoom

	// Seven-Star Convenience Store
	sevenStar := world.SpawnSpace(
		"seven-star-mart",
		"Seven-Star Convenience Store",
		"Blinding fluorescent aisles stocked with hot canned coffee, refrigerated onigiri, and cheap umbrella racks.",
		true,
	)
	locs.SevenStarMart = sevenStar

	// University Metro Station Underpass
	metroUnderpass := world.SpawnSpace(
		"metro-underpass",
		"University Metro Station Underpass",
		"A white-tiled subterranean concourse with automated ticket gates, buskers playing acoustic guitar, and drafty transit breezes.",
		true,
	)
	locs.MetroUnderpass = metroUnderpass

	// Portals connecting off-campus transit network
	_, _ = world.SpawnPortal("quad-to-metro", "Metro Station Stairs", housingQuad.ID, metroUnderpass.ID, "down into the subway underpass")
	_, _ = world.SpawnPortal("metro-to-quad", "Exit to Campus Quad", metroUnderpass.ID, housingQuad.ID, "up to Bryn Hall quad")

	_, _ = world.SpawnPortal("metro-to-cyber", "Alley Exit to CyberNet", metroUnderpass.ID, cyberNet.ID, "around the corner to CyberNet Cafe")
	_, _ = world.SpawnPortal("cyber-to-metro", "Subway Station Entrance", cyberNet.ID, metroUnderpass.ID, "into the metro station")

	_, _ = world.SpawnPortal("metro-to-mart", "Corner Crossing to Seven-Star", metroUnderpass.ID, sevenStar.ID, "across to the convenience store")
	_, _ = world.SpawnPortal("mart-to-metro", "Metro Underpass Entry", sevenStar.ID, metroUnderpass.ID, "down into subway underpass")

	_, _ = world.SpawnPortal("mart-to-laundromat", "East Avenue Sidewalk", sevenStar.ID, laundromat.ID, "down the block to the laundromat")
	_, _ = world.SpawnPortal("laundromat-to-mart", "Avenue to Seven-Star", laundromat.ID, sevenStar.ID, "up to the corner market")

	_, _ = world.SpawnPortal("mart-to-hotel", "Side Alley to Hotel Starlet", sevenStar.ID, hotelRoom.ID, "up the narrow hotel stairs to Room 204")
	_, _ = world.SpawnPortal("hotel-to-mart", "Hotel Exit", hotelRoom.ID, sevenStar.ID, "downstairs to the street")

	// =========================================================================
	// 5. DETAILED PROPS & EVIDENCE (CLUTTER, SECRETS & AFFORDANCES)
	// =========================================================================

	// In Living Room:
	couch, _ := world.SpawnEntity("living-room-couch", "Secondhand Corduroy Couch", "A worn camel-brown three-seater with slightly flattened cushions.", livingRoom.ID, sim.Link{Relation: sim.RelOn})
	couch.Salience = 0.80

	coffeeTable, _ := world.SpawnEntity("coffee-table", "Low Wooden Coffee Table", "A small pine table placed directly in front of the couch.", livingRoom.ID, sim.Link{Relation: sim.RelOn})
	locs.CoffeeTable = coffeeTable

	notes, _ := world.SpawnEntity(
		"relationship-notes",
		"Color-Coded Relationship Notes",
		"Neatly handwritten, color-tabbed index cards covering fake anniversaries, couple quirks, first-date lore, and pet peeves.",
		coffeeTable.ID,
		sim.Link{Relation: sim.RelOn, Placement: "stacked prominently on the table"},
	)
	notes.Salience = 0.95
	_ = notes.Attributes.Set("Handwriting", "Meticulous, four-color gel pen ink, slightly trembling margins")
	locs.RelationshipNotes = notes

	// The Hidden Audit Notice (Private ACL: only Nelly knows it's hidden under the design magazine)
	auditLetter, _ := world.SpawnEntity(
		"housing-audit-letter",
		"University Housing Inspection Notice",
		"An official notification on heavy cardstock: 'MANDATORY IN-PERSON CO-TENANCY VERIFICATION: Thursday Oct 22 at 18:30'.",
		coffeeTable.ID,
		sim.Link{Relation: sim.RelOn, Placement: "tucked discreetly beneath an oversized architecture magazine"},
	)
	auditLetter.Salience = 0.90
	_ = auditLetter.Attributes.SetWithVisibility("Penalty Clause", "Failure to verify co-tenancy results in immediate lease revocation and $6,800 retroactive market back-charges.", sim.VisibilityPrivate, []sim.EntityID{"nelly"})
	locs.AuditLetter = auditLetter

	chair, _ := world.SpawnEntity("dining-chair", "Wooden Dining Chair", "A mismatched wooden chair near the kitchen counter.", livingRoom.ID, sim.Link{Relation: sim.RelOn})
	flannel, _ := world.SpawnEntity(
		"staged-flannel",
		"Oversized Guy's Flannel Shirt",
		"A large plaid flannel shirt draped deliberately over the chair back to signal a resident boyfriend.",
		chair.ID,
		sim.Link{Relation: sim.RelOn, Placement: "carefully draped to look casual"},
	)
	flannel.Salience = 0.70
	locs.StagedFlannel = flannel

	bookshelf, _ := world.SpawnEntity("living-bookshelf", "MDF Bookshelf", "Tall laminate bookshelf packed with textbooks, sketchpads, and ceramics.", livingRoom.ID, sim.Link{Relation: sim.RelOn})
	photo, _ := world.SpawnEntity(
		"photoshop-couple-photo",
		"Framed Orchard Photo",
		"A cheap white plastic frame showing a laughably bad Photoshop collage of Zack and Nelly standing together at an apple orchard.",
		bookshelf.ID,
		sim.Link{Relation: sim.RelOn, Placement: "resting prominently at eye level"},
	)
	photo.Salience = 0.85
	_ = photo.Attributes.Set("Quality", "Uneven lighting, obvious cutout edges, printed on glossy photo paper twenty minutes ago")
	locs.PhotoshopPhoto = photo

	// In Kitchenette:
	refrigerator, _ := world.SpawnEntity("refrigerator", "College-Dorm Refrigerator", "A noisy white fridge covered in takeout menus and magnetic poetry. The door is currently cracked open.", kitchenette.ID, sim.Link{Relation: sim.RelOn})
	refrigerator.Salience = 0.80
	_ = refrigerator.Attributes.Set("Contents", "Half-empty tub of garlic hummus, jar of Greek olives, one lemon, carton of almond milk")

	// On Balcony:
	lavenderPot, _ := world.SpawnEntity("lavender-pot", "Half-Wilted Potted Lavender", "A terracotta pot containing dry, pale purple lavender.", balcony.ID, sim.Link{Relation: sim.RelOn})
	lavenderPot.Salience = 0.65
	_ = lavenderPot.Attributes.Set("History", "Bought by David five months ago at the Sunday farmers market; struggling to survive")

	// In Guest Bathroom:
	toothbrushHolder, _ := world.SpawnEntity("guest-toothbrush-holder", "Ceramic Toothbrush Mug", "A simple blue ceramic mug beside the sink.", guestBath.ID, sim.Link{Relation: sim.RelOn})
	toothbrush, _ := world.SpawnEntity(
		"staged-toothbrush",
		"Brand-New Navy Toothbrush",
		"An extra toothbrush standing upright in the mug. The bristles are bone-dry and the torn plastic packaging is stuffed in the wastebasket.",
		toothbrushHolder.ID,
		sim.Link{Relation: sim.RelIn},
	)
	toothbrush.Salience = 0.75
	locs.StagedToothbrush = toothbrush

	// In Nelly's Bedroom:
	desk, _ := world.SpawnEntity("nelly-study-desk", "Organized Study Desk", "A wooden desk covered with neat stacks of drafting paper and pencil cases.", nellyBedroom.ID, sim.Link{Relation: sim.RelOn})
	_, _ = world.SpawnEntity("gel-pen-set", "Four-Color Gel Pen Set", "Fine-tip colored pens used for color-coding notes.", desk.ID, sim.Link{Relation: sim.RelOn})

	closet, _ := world.SpawnEntity("bedroom-closet", "Built-In Wardrobe Closet", "A narrow closet with sliding doors.", nellyBedroom.ID, sim.Link{Relation: sim.RelOn})
	_, _ = world.SpawnEntity("empty-hangers", "Cluster of Empty Wire Hangers", "Twelve bare wire hangers pushed to the left side of the rail where David's jackets used to hang.", closet.ID, sim.Link{Relation: sim.RelIn})

	return locs, nil
}
