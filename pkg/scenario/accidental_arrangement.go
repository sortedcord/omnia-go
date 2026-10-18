package scenario

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"omnia.adityagupta.dev/pkg/actor"
	"omnia.adityagupta.dev/pkg/llm"
	"omnia.adityagupta.dev/pkg/sim"
)

// LoadDotEnv loads key-value pairs from a .env file if present.
func LoadDotEnv(filename string) {
	file, err := os.Open(filename)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			k := strings.TrimSpace(parts[0])
			v := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
			if os.Getenv(k) == "" {
				_ = os.Setenv(k, v)
			}
		}
	}
}

// ScenarioState contains all initialized entities and minds for the scenario.
type ScenarioState struct {
	World       *sim.Simulation
	Client      llm.Client
	HumanCtrl   *sim.HumanController // Non-nil when interactive
	ZackEntity  *sim.Entity
	ZackMind    *actor.Actor
	NellyEntity *sim.Entity
	NellyMind   *actor.Actor
	MiaEntity   *sim.Entity
	MiaMind     *actor.Actor
	LivingRoom  *sim.Entity
	Kitchen     *sim.Entity
	Locations   *WorldLocations
}

// Setup initializes the "Accidental Arrangement" slice-of-life scenario with full 3-tier memory depth.
func Setup(interactive bool) (*ScenarioState, error) {
	LoadDotEnv(".env")

	apiKey := os.Getenv("OMNIA_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("OMNIA_API_KEY (or OPENAI_API_KEY) must be set in environment or .env file")
	}

	apiURL := os.Getenv("OMNIA_API_URL")
	if apiURL == "" {
		apiURL = "https://ai.adityagupta.dev/v1"
	}

	model := "gemini-3.5-flash-lite"
	client := llm.NewHTTPClient(apiURL, apiKey)

	// 1. Initialize Simulation World (Evening in University Housing)
	world := sim.New(sim.Clock{
		Cycle:        1,
		StartTime:    time.Date(2026, 10, 16, 18, 35, 0, 0, time.UTC),
		TickDuration: 20 * time.Second,
		TickCount:    1,
	}, sim.WithSeed(42))

	// 2. Spatial World Topology (22 Detailed Interconnected Spaces & Props)
	locs, err := BuildWorldTopology(world)
	if err != nil {
		return nil, fmt.Errorf("failed building world topology: %w", err)
	}

	// 3. Create the Actors with 3-Tier Subjective Memory

	// --- ZACK (Player Character / Reluctant Partner) ---
	zackProfile := actor.Profile{
		ID:          "zack",
		Name:        "Zack",
		Personality: "Observant, grounded, dry understated humor, fiercely self-reliant, quietly exhausted from months of couch-surfing and internet cafes",
		Directives: []string{
			"Figure out what Mia and Nelly actually expect you to do before agreeing to anything crazy",
			"React naturally to Nelly's awkwardness without making her completely combust",
			"Protect your dignity—do not let anyone treat you like a charity case",
		},
		Background: "Fourth-year architecture student. After his roommates moved out with their partners, he lost his lease and spent months couch-surfing, sleeping in internet cafes, and occasionally freezing on park benches. He does not know Nelly prior to Mia dragging him into Apt 3B.",
	}
	zackMind := actor.New(zackProfile)
	SeedZackEpisodes(zackMind)

	zackEntity, _ := world.SpawnActor(
		"zack",
		zackMind,
		locs.Breezeway3F.ID,
		"A tall college student in a worn canvas jacket, holding a paper coffee cup and standing on the breezeway looking mildly bewildered and sleep-deprived.",
	)
	_ = zackEntity.Attributes.Set("mood", "Mildly bewildered, dryly amused, assessing the situation")
	_ = zackEntity.Attributes.Set("arousal", "normal")
	_ = zackEntity.Attributes.Set("Sleep Debt", "Severe (weeks of internet cafe chairs)")
	_ = zackEntity.Attributes.SetWithVisibility("Internal Sarcasm", "Wondering what impossible scheme Mia has cooked up behind this door", sim.VisibilityPrivate, []sim.EntityID{"zack"})

	jacket, _ := world.SpawnEntity("zack-jacket", "Canvas Jacket", "Everyday casual zip-up jacket worn thin at the elbows.", zackEntity.ID, sim.Link{Relation: sim.RelWorn})
	pocket := world.SpawnSpace("jacket-pocket", "Jacket Pocket", "Small front pocket.", false)
	_ = world.Move(pocket.ID, jacket.ID, sim.Link{Relation: sim.RelIn})
	_, _ = world.SpawnEntity("zack-phone", "Smartphone", "Screen showing Mia's frantic texts: 'GET UP HERE NOW; YOU WILL THANK ME'.", pocket.ID, sim.Link{Relation: sim.RelIn})

	var humanCtrl *sim.HumanController
	if interactive {
		humanCtrl = sim.NewHumanController(nil, model)
		_ = world.SetController(zackEntity.ID, humanCtrl)
	}

	// --- NELLY (Autonomous AI Character / Panicked Tenant) ---
	nellyProfile := actor.Profile{
		ID:          "nelly",
		Name:        "Nelly",
		Personality: "Warm, considerate, severely flustered under relational pressure, overthinks social perception, detail-obsessed when stressed, terrified of administrative failure",
		Directives: []string{
			"Try desperately to act natural and explain the fake relationship setup without dying of embarrassment",
			"Keep the subsidized couple apartment so you don't get evicted by the housing board and forced into your parents' basement",
			"Figure out if Zack can be trusted or if he thinks you're completely insane",
		},
		Background: "Lives in subsidized couple housing (Apt 3B). Her ex David walked out six weeks ago. The housing board scheduled a mandatory in-person audit for Thursday. Mia dragged Zack in as a stand-in boyfriend.",
	}
	nellyMind := actor.New(nellyProfile)
	SeedNellyEpisodes(nellyMind)

	nellyEntity, _ := world.SpawnActor(
		"nelly",
		nellyMind,
		locs.LivingRoom.ID,
		"A girl in her early 20s wearing an oversized gray hoodie pulled over her knuckles, hair hastily pinned back, looking equal parts mortified and determined.",
	)
	nellyEntity.Salience = 0.95
	_ = nellyEntity.Attributes.Set("mood", "Deeply flustered, overthinking every syllable, gripping hoodie sleeves")
	_ = nellyEntity.Attributes.Set("arousal", "high")
	_ = nellyEntity.Attributes.Set("Heart Rate", "125 bpm")
	_ = nellyEntity.Attributes.Set("Social Battery", "12%")
	_ = nellyEntity.Attributes.Set("Face Redness", "Maximum")
	_ = nellyEntity.Attributes.SetWithVisibility("Subconscious Panic", "Desperately terrified of having to move back into her parents' basement", sim.VisibilityPrivate, []sim.EntityID{"nelly"})

	// --- MIA (Autonomous AI Character / The Mastermind Fixer) ---
	miaProfile := actor.Profile{
		ID:          "mia",
		Name:        "Mia",
		Personality: "Socially bulletproof, cheerful, pragmatically shameless, sharp-eyed fixer, fiercely protective of her friends behind an amused smile",
		Directives: []string{
			"Keep Zack and Nelly moving forward so the housing inspection doesn't blow up",
			"Ensure Zack realizes this solves his housing crisis too while letting him keep his pride",
			"Enjoy the awkward entertainment while raiding Nelly's fridge",
		},
		Background: "Close friend of both Zack and Nelly (who didn't know each other). Discovered Zack sleeping in a laundromat and Nelly crying on her kitchen floor, and engineered this arrangement as the ultimate pragmatic win-win.",
	}
	miaMind := actor.New(miaProfile)
	SeedMiaEpisodes(miaMind)

	miaEntity, _ := world.SpawnActor(
		"mia",
		miaMind,
		locs.Breezeway3F.ID,
		"A chic, confident girl leaning nonchalantly against the metal breezeway railing right outside the door to Apartment 3B, sipping an iced tea with a satisfied smirk.",
	)
	miaEntity.Salience = 0.75
	_ = miaEntity.Attributes.Set("mood", "Cheerfully detached, expectant, highly entertained")
	_ = miaEntity.Attributes.Set("arousal", "normal")
	_ = miaEntity.Attributes.Set("Tactical Satisfaction", "95%")
	_ = miaEntity.Attributes.Set("Appetite", "Snack hunting")

	// 4. Seed Tier 3 Subjective Reality Dossiers & Asymmetric Relational Models
	SeedDossiers(zackMind, nellyMind, miaMind)

	// 5. Schedule Narrative Opening Events
	// Outside the door: Zack arrives and finds Mia waiting for him
	world.ScheduleEvent(1, locs.Breezeway3F.ID,
		"You climb the concrete stairs to the third floor of Bryn Hall, following the apartment address Mia frantically texted you twenty minutes ago. "+
			"Mia is already leaning against the metal breezeway railing right outside the closed door to Apartment 3B, casually swirling an iced tea with an amused grin. "+
			"\"Took you long enough, slowpoke,\" she says quietly, nodding toward the door of 3B. "+
			"\"Before we head in, quick ground rules: Nelly is inside having a full-blown existential crisis over her lease audit, and you are about to be her knight in slightly rumpled armor.\"")

	// Inside the apartment: Nelly waits nervously, hearing voices on the breezeway
	world.ScheduleEvent(1, locs.LivingRoom.ID,
		"Inside the quiet living room of Apt 3B, you pace anxiously between the coffee table and the front door, clutching your oversized hoodie cuffs over your knuckles. "+
			"The color-coded relationship index cards are spread neatly across the pine table. "+
			"From out on the breezeway, you hear footsteps and Mia's muffled voice talking to someone outside the door. "+
			"Your stomach drops into freefall—she actually brought someone.")

	return &ScenarioState{
		World:       world,
		Client:      client,
		HumanCtrl:   humanCtrl,
		ZackEntity:  zackEntity,
		ZackMind:    zackMind,
		NellyEntity: nellyEntity,
		NellyMind:   nellyMind,
		MiaEntity:   miaEntity,
		MiaMind:     miaMind,
		LivingRoom:  locs.LivingRoom,
		Kitchen:     locs.Kitchenette,
		Locations:   locs,
	}, nil
}
