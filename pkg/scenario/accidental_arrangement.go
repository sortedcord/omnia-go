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
}

// Setup initializes the "Accidental Arrangement" slice-of-life scenario.
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

	// 2. Spatial Topology (Nelly's Subsidized Couple Apartment)
	livingRoom := world.SpawnSpace(
		"living-room",
		"Nelly's Apartment - Living Room",
		"A warm, slightly cluttered college apartment living room. Everything has been hastily staged to look like a loving couple lives here.",
		true,
	)
	_ = livingRoom.Attributes.Set("Atmosphere", "Warm, faintly smelling of lavender, coffee, and printed paper")
	_ = livingRoom.Attributes.Set("Social Tension", "Thick, awkward relational panic")

	kitchen := world.SpawnSpace(
		"kitchenette",
		"Open Kitchenette",
		"A compact kitchen with laminate countertops and a sink, separated from the living room by an open breakfast counter.",
		true,
	)

	// Open connection between living room and kitchen
	_, _ = world.SpawnPortal("counter-pass", "Kitchen Breakfast Counter", livingRoom.ID, kitchen.ID, "kitchen counter")
	_, _ = world.SpawnPortal("counter-pass-return", "Breakfast Counter to Living Room", kitchen.ID, livingRoom.ID, "into living room")

	// 3. Staged Props & Clutter
	coffeeTable, _ := world.SpawnEntity(
		"coffee-table",
		"Low Wooden Coffee Table",
		"A small pine table placed directly in front of the couch.",
		livingRoom.ID,
		sim.Link{Relation: sim.RelOn},
	)

	_, _ = world.SpawnEntity(
		"relationship-notes",
		"Color-Coded Relationship Notes",
		"Neatly handwritten, color-tabbed index cards covering fake anniversaries, couple quirks, first-date lore, and pet peeves.",
		coffeeTable.ID,
		sim.Link{Relation: sim.RelOn, Placement: "stacked prominently on the table"},
	)

	chair, _ := world.SpawnEntity(
		"dining-chair",
		"Wooden Dining Chair",
		"A mismatched wooden chair near the kitchen counter.",
		livingRoom.ID,
		sim.Link{Relation: sim.RelOn},
	)

	_, _ = world.SpawnEntity(
		"staged-flannel",
		"Oversized Guy's Flannel Shirt",
		"A large plaid flannel shirt draped deliberately over the chair back to signal a resident boyfriend.",
		chair.ID,
		sim.Link{Relation: sim.RelOn, Placement: "carefully draped to look casual"},
	)

	_, _ = world.SpawnEntity(
		"refrigerator",
		"College-Dorm Refrigerator",
		"A noisy white fridge covered in takeout menus and magnetic poetry. The door is currently cracked open.",
		kitchen.ID,
		sim.Link{Relation: sim.RelOn},
	)

	// 4. Create the Actors

	// --- ZACK (Player Character) ---
	zackProfile := actor.Profile{
		ID:          "zack",
		Name:        "Zack",
		Personality: "Observant, grounded, slightly dry humor, loyal friend caught in an absurd social mess",
		Directives: []string{
			"Decide how to handle Mia's chaotic scheme and Nelly's housing emergency",
			"React naturally to Nelly's awkwardness without making her completely combust",
		},
	}
	zackMind := actor.New(zackProfile)

	// Seed Zack's initial thoughts
	zackMind.AddThought(actor.Thought{
		Kind:     actor.ThoughtIntention,
		Content:  "Figure out what Mia and Nelly actually expect me to do before I agree to anything crazy.",
		Links:    []string{"living-room", "relationship-notes"},
		Salience: 0.90,
	})
	zackMind.AddThought(actor.Thought{
		Kind:     actor.ThoughtBelief,
		Content:  "Nelly looks like she is three seconds away from spontaneous social combustion.",
		Links:    []string{"living-room"},
		Salience: 0.80,
	})
	zackMind.AddThought(actor.Thought{
		Kind:     actor.ThoughtSuspicion,
		Content:  "Mia is 100% going to leave me to deal with this awkwardness while she raids the fridge.",
		Links:    []string{"kitchenette"},
		Salience: 0.75,
	})

	zackEntity, _ := world.SpawnActor("zack", zackMind, livingRoom.ID, "A college student in casual clothes, standing near the doorway looking mildly bemused.")
	_ = zackEntity.Attributes.Set("mood", "Mildly bewildered, dryly amused, assessing the disaster")
	_ = zackEntity.Attributes.Set("arousal", "normal")
	_ = zackEntity.Attributes.Set("Awkwardness Bar", "Rising")
	// Private attribute accessible only to Zack:
	_ = zackEntity.Attributes.SetWithVisibility("Internal Sarcasm", "Barely keeping a straight face", sim.VisibilityPrivate, []sim.EntityID{"zack"})

	jacket, _ := world.SpawnEntity("zack-jacket", "Canvas Jacket", "Everyday casual zip-up jacket.", zackEntity.ID, sim.Link{Relation: sim.RelWorn})
	pocket := world.SpawnSpace("jacket-pocket", "Jacket Pocket", "Small front pocket.", false)
	_ = world.Move(pocket.ID, jacket.ID, sim.Link{Relation: sim.RelIn})
	_, _ = world.SpawnEntity("zack-phone", "Smartphone", "Screen showing Mia's frantic texts: 'GET UP HERE NOW'.", pocket.ID, sim.Link{Relation: sim.RelIn})

	var humanCtrl *sim.HumanController
	if interactive {
		humanCtrl = sim.NewHumanController(nil, model)
		_ = world.SetController(zackEntity.ID, humanCtrl)
	}

	// --- NELLY (Autonomous AI Character) ---
	nellyProfile := actor.Profile{
		ID:          "nelly",
		Name:        "Nelly",
		Personality: "Warm, considerate, severely flustered under relational pressure, overthinks social perception, detail-obsessed when stressed",
		Directives: []string{
			"Try desperately to act natural and explain the fake relationship setup without dying of embarrassment",
			"Keep the subsidized couple apartment so you don't get evicted by the housing board",
		},
		Background: "Lives in subsidized couple housing. Her ex moved out months ago, but she couldn't afford to report it. Verification is approaching, and Mia dragged Zack in as her temporary fake boyfriend.",
	}
	nellyMind := actor.New(nellyProfile)

	nellyMind.AddThought(actor.Thought{
		Kind:     actor.ThoughtIntention,
		Content:  "Explain the arrangement to Zack so he understands the stakes, but stop sounding so formal and weird.",
		Links:    []string{"relationship-notes", "living-room"},
		Salience: 0.95,
	})
	nellyMind.AddThought(actor.Thought{
		Kind:     actor.ThoughtBelief,
		Content:  "The 'notes' comment made me sound completely insane. I need to make a normal human face.",
		Links:    []string{"living-room"},
		Salience: 0.90,
	})
	nellyMind.AddThought(actor.Thought{
		Kind:     actor.ThoughtSuspicion,
		Content:  "Mia is hiding in the kitchen on purpose to make me do the talking.",
		Links:    []string{"kitchenette"},
		Salience: 0.70,
	})
	nellyMind.AddThought(actor.Thought{
		Kind:     actor.ThoughtFeeling,
		Content:  "My cheeks feel like boiling radiators and my fingers are trembling inside my sleeves.",
		Links:    []string{"living-room"},
		Salience: 0.85,
	})

	nellyEntity, _ := world.SpawnActor("nelly", nellyMind, livingRoom.ID,
		"A girl in her early 20s wearing an oversized gray hoodie pulled over her knuckles, hair hastily pinned back, looking equal parts mortified and determined.")
	nellyEntity.Salience = 0.95

	_ = nellyEntity.Attributes.Set("mood", "Deeply flustered, overthinking every syllable, gripping hoodie sleeves")
	_ = nellyEntity.Attributes.Set("arousal", "high")
	_ = nellyEntity.Attributes.Set("Heart Rate", "125 bpm")
	_ = nellyEntity.Attributes.Set("Social Battery", "12%")
	_ = nellyEntity.Attributes.Set("Face Redness", "Maximum")
	_ = nellyEntity.Attributes.SetWithVisibility("Subconscious Panic", "Desperately terrified of having to move back into her parents' basement", sim.VisibilityPrivate, nil)

	// --- MIA (Autonomous AI Character) ---
	miaProfile := actor.Profile{
		ID:          "mia",
		Name:        "Mia",
		Personality: "Socially bulletproof, cheerful, pragmatically shameless, smooth, easily amused, loves placing people together to solve problems",
		Directives: []string{
			"Keep Zack and Nelly moving forward so the housing inspection doesn't blow up",
			"Enjoy the awkward entertainment while raiding Nelly's fridge",
		},
		Background: "Close friend of both Zack and Nelly. Sees zero moral issue with roping Zack into a fake relationship to save Nelly's lease.",
	}
	miaMind := actor.New(miaProfile)

	miaMind.AddThought(actor.Thought{
		Kind:     actor.ThoughtIntention,
		Content:  "Give Zack and Nelly space to break the ice while I find something good to eat.",
		Links:    []string{"kitchenette", "refrigerator"},
		Salience: 0.80,
	})
	miaMind.AddThought(actor.Thought{
		Kind:     actor.ThoughtBelief,
		Content:  "Zack is too nice to leave Nelly hanging, and Nelly's panic is adorable.",
		Links:    []string{"living-room"},
		Salience: 0.85,
	})

	miaEntity, _ := world.SpawnActor("mia", miaMind, kitchen.ID,
		"A chic, confident girl leaning nonchalantly against the open refrigerator door, sipping an iced tea.")
	miaEntity.Salience = 0.75

	_ = miaEntity.Attributes.Set("mood", "Cheerfully detached, hungry, highly entertained")
	_ = miaEntity.Attributes.Set("arousal", "normal")
	_ = miaEntity.Attributes.Set("Stress Level", "0%")
	_ = miaEntity.Attributes.Set("Appetite", "Snack hunting")

	// Schedule initial narrative opening event
	world.ScheduleEvent(1, livingRoom.ID,
		"Mia pulled you into the apartment and immediately wandered off toward the kitchen. "+
			"Nelly has just emerged from the hallway, clutching the cuffs of her oversized hoodie. "+
			"Her eyes lock onto yours, her face goes bright pink, and she blurts out: \"I made notes.\" "+
			"There is a suffocatingly awkward three-second pause. \"...I hear how bad that sounds now.\" "+
			"From the kitchen, Mia opens the fridge with a casual clatter, totally unbothered.")

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
		LivingRoom:  livingRoom,
		Kitchen:     kitchen,
	}, nil
}
