package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"omnia.adityagupta.dev/internal/env"
	"omnia.adityagupta.dev/pkg/llm"
	"omnia.adityagupta.dev/pkg/scenario"
	"omnia.adityagupta.dev/pkg/sim"
)

func main() {
	playMode := flag.Bool("play", true, "Play interactively as player via terminal (defaults to true)")
	httpAddr := flag.String("http", "", "HTTP listen address for interactive play mode (e.g. :8080 or 127.0.0.1:8080)")
	maxTicks := flag.Int("ticks", 4, "Maximum number of ticks to simulate")
	scenarioPath := flag.String("scenario", "scenarios/accidental_arrangement.omnia.json", "Path to scenario JSON specification")
	startID := flag.String("start", "", "Specific start point ID (empty uses first start)")
	playerID := flag.String("player", "", "Character ID for player (empty uses authored default)")
	flag.Parse()

	env.LoadDotEnv(".env")

	apiKey := os.Getenv("OMNIA_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}
	if apiKey == "" {
		fmt.Fprintf(os.Stderr, "Error: OMNIA_API_KEY (or OPENAI_API_KEY) must be set in environment or .env file\n")
		os.Exit(1)
	}

	apiURL := os.Getenv("OMNIA_API_URL")
	if apiURL == "" {
		apiURL = "https://ai.adityagupta.dev/v1"
	}
	model := "gemini-3.5-flash-lite"
	client := llm.NewHTTPClient(apiURL, apiKey)

	f, err := os.Open(*scenarioPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening scenario file %q: %v\n", *scenarioPath, err)
		os.Exit(1)
	}
	defer f.Close()

	doc, err := scenario.Decode(f)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error decoding scenario %q: %v\n", *scenarioPath, err)
		os.Exit(1)
	}

	isInteractive := *playMode || *httpAddr != ""
	inst, err := scenario.Build(doc, scenario.BuildOptions{
		Client:      client,
		Model:       model,
		Start:       *startID,
		Player:      *playerID,
		Interactive: isInteractive,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error building scenario instance: %v\n", err)
		os.Exit(1)
	}

	var httpServer *sim.HTTPServer
	if *httpAddr != "" {
		httpServer = sim.NewHTTPServer(inst.World, inst.HumanCtrl, *httpAddr)
		if err := httpServer.Start(context.Background()); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to start HTTP server: %v\n", err)
			os.Exit(1)
		}
		listenAddr := httpServer.Addr()
		if strings.HasPrefix(listenAddr, ":") {
			listenAddr = "http://127.0.0.1" + listenAddr
		}
		fmt.Printf("\n[HTTP Server Online] Endpoints active at %s (/turn, /action, /status, /events)\n", listenAddr)
	} else if *playMode && inst.HumanCtrl != nil {
		// Terminal TTY Play Mode: scan standard input
		go func() {
			scanner := bufio.NewScanner(os.Stdin)
			for turn := range inst.HumanCtrl.Turns() {
				fmt.Println("\n==================================================")
				fmt.Printf(">>> YOUR TURN: %s [%s] <<<\n", turn.ActorName, turn.Timestamp)
				fmt.Println("==================================================")
				if turn.Thoughts != "" {
					fmt.Println(turn.Thoughts)
					fmt.Println()
				}
				fmt.Println(turn.Stimulus)
				fmt.Print("\nWhat do you say or do? (type your action/speech or 'quit')\n> ")
				if !scanner.Scan() {
					inst.HumanCtrl.Quit()
					return
				}
				input := strings.TrimSpace(scanner.Text())
				if input == "quit" || input == "exit" {
					inst.HumanCtrl.Quit()
					return
				}
				inst.HumanCtrl.Reply(input)
			}
		}()
	}

	// 5. Verification & Opening Scene Setup
	playerLocName := "Unknown"
	if playerEnt, ok := inst.World.Entities[inst.Player]; ok {
		if loc, ok := inst.World.Entities[playerEnt.Parent]; ok {
			playerLocName = loc.Name
		}
	}

	var coStarNames []string
	for aID, a := range inst.Actors {
		if aID != inst.Player {
			coStarNames = append(coStarNames, a.Profile.Name)
		}
	}
	slices.Sort(coStarNames)

	playerName := string(inst.Player)
	if act, ok := inst.Actors[inst.Player]; ok && act.Profile.Name != "" {
		playerName = fmt.Sprintf("%s (%s)", act.Profile.Name, inst.Player)
	}

	fmt.Println("==================================================")
	fmt.Printf("   SCENARIO: %s\n", strings.ToUpper(inst.Doc.Meta.Title))
	fmt.Println("==================================================")
	fmt.Printf("Setting:   %s\n", playerLocName)
	fmt.Printf("Player:    %s\n", playerName)
	fmt.Printf("Co-Stars:  %s\n", strings.Join(coStarNames, ", "))
	fmt.Printf("Clock:     %s\n", inst.World.Clock)

	// 6. Run the Simulation Loop
	ctxTimeout := 180 * time.Second
	if isInteractive {
		ctxTimeout = 25 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), ctxTimeout)
	defer cancel()

	fmt.Println("\n==================================================")
	if *httpAddr != "" {
		fmt.Println("       INTERACTIVE HTTP MODE: CONTROL VIA CURL    ")
	} else if *playMode {
		fmt.Printf("       INTERACTIVE MODE: YOU ARE PLAYING %s\n", inst.Player)
	} else {
		fmt.Println("       AUTONOMOUS DEMONSTRATION MODE              ")
	}
	fmt.Println("==================================================")

	_, simErr := inst.World.Run(ctx, *maxTicks, func(report sim.TickReport) {
		targetsInfo := ""
		if len(report.Reaction.Action.Targets) > 0 {
			targetsInfo = fmt.Sprintf(" -> [%s]", strings.Join(report.Reaction.Action.Targets, ", "))
		}

		if *httpAddr != "" && report.ActorID == inst.Player {
			fmt.Printf("\n[Player Action (%s)] (%s%s): %s\n",
				inst.Player, report.Reaction.Action.Type, targetsInfo, report.Reaction.Action.Action)
			if report.SceneRuling != nil {
				fmt.Printf("[Scene Resolution (%s)]: %s\n",
					report.SceneRuling.SceneID, report.SceneRuling.Resolution)
			}
			return
		}

		fmt.Printf("\n>>> [TICK %d | %s | Scene: %s] Turn: %s (%s) <<<\n",
			report.Tick, report.Timestamp, report.SceneID, report.ActorName, report.ActorID)

		if report.Reaction.Cognition != "" {
			fmt.Printf("\n--- Private Cognition (Thinking Tokens) ---\n%s\n", report.Reaction.Cognition)
		}

		fmt.Printf("\n--- Manifest Turn ---\nType:   %s%s\nAction: %s\n",
			report.Reaction.Action.Type, targetsInfo, report.Reaction.Action.Action)

		if report.SceneRuling != nil {
			fmt.Printf("\n--- Scene Arbiter Ruling (%s) ---\nResolution:   %s\n",
				report.SceneRuling.SceneID, report.SceneRuling.Resolution)
			for _, v := range report.SceneRuling.Verdicts {
				if v.ActorID == report.ActorID {
					fmt.Printf("Verdict:      %s (outcome: %s)\n", v.Verdict, v.Outcome)
					if v.RemainingIntent != "" {
						fmt.Printf("Remaining:    %s\n", v.RemainingIntent)
					}
					break
				}
			}
			if len(report.SceneRuling.AppliedEffects) > 0 {
				fmt.Printf("Mutations:    %d applied\n", len(report.SceneRuling.AppliedEffects))
			}
		}

		if report.Recall != nil && report.Recall.SelfDossier != nil && len(report.Recall.SelfDossier.Claims) > 0 {
			fmt.Printf("\n--- State of Mind (Dossier Claims) ---\n")
			for _, c := range report.Recall.SelfDossier.Claims {
				fmt.Printf("  * [%-9s | salience: %.2f] %s\n", c.Kind, c.Salience, c.Content)
			}
		} else if len(report.Thoughts) > 0 {
			fmt.Printf("\n--- State of Mind (Selected Thoughts) ---\n")
			for _, th := range report.Thoughts {
				fmt.Printf("  * [%s:%-9s | salience: %.2f] %s\n", th.ID, th.Kind, th.Salience, th.Content)
			}
		}

		if report.PriorDistill != nil && len(report.PriorDistill.Applied) > 0 {
			fmt.Printf("\n--- Thought Distiller Updates ---\n")
			for _, op := range report.PriorDistill.Applied {
				fmt.Printf("  * [%s] id: %s (%s): %q\n", op.Op, op.ID, op.Kind, op.Content)
			}
		}

		fmt.Printf("\nTurn Metrics: %v | %d tokens\n",
			report.Reaction.Duration.Round(time.Millisecond), report.Reaction.Usage.TotalTokens)
		fmt.Println("--------------------------------------------------")
	})

	if httpServer != nil {
		httpServer.SetGameOver(simErr)
	}

	if simErr != nil && !errors.Is(simErr, sim.ErrQuit) {
		fmt.Printf("Simulation ended with error: %v\n", simErr)
		return
	}

	// 7. Final Event Log Summary
	fmt.Println("\n==================================================")
	fmt.Println("               SIMULATION EVENT LOG               ")
	fmt.Println("==================================================")
	for i, ev := range inst.World.GetEventLog() {
		fmt.Printf("[%d] Tick %d | Kind: %-7s | Source: %-15s | Loc: %s\n    %s\n",
			i+1, ev.Tick, ev.Kind, ev.SourceName, ev.LocationID, ev.Description)
	}

	// 8. Final Dossiers & Mind State
	fmt.Println("\n==================================================")
	fmt.Println("              EVOLVED DOSSIERS & MIND STATE       ")
	fmt.Println("==================================================")
	var actorOrder []sim.EntityID
	for aID := range inst.Actors {
		actorOrder = append(actorOrder, aID)
	}
	slices.Sort(actorOrder)

	for _, aID := range actorOrder {
		act := inst.Actors[aID]
		if act.Mind == nil {
			continue
		}
		fmt.Printf("\n>>> %s (%s) <<<\n", act.Profile.Name, aID)
		for _, d := range act.Mind.DossiersSnapshot() {
			if len(d.Claims) > 0 || d.Impression != "" {
				subjName := d.SubjectID
				if s, ok := act.Mind.GetSubject(d.SubjectID); ok && len(s.Names) > 0 {
					subjName = s.Names[0]
				}
				fmt.Printf("\n[Subject: %s (%s)]\n", subjName, d.SubjectID)
				if d.Impression != "" {
					fmt.Printf("  Impression: %s\n", d.Impression)
				}
				for _, c := range d.Claims {
					fmt.Printf("  - [%-9s | salience: %.2f | conf: %.2f] %s\n", c.Kind, c.Salience, c.Conviction, c.Content)
				}
			}
		}
	}
}
