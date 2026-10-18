package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"omnia.adityagupta.dev/internal/env"
	"omnia.adityagupta.dev/pkg/llm"
	"omnia.adityagupta.dev/pkg/scenario"
	"omnia.adityagupta.dev/pkg/sim"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP server listen address (e.g. :8080 or 127.0.0.1:8080)")
	maxTicks := flag.Int("ticks", 10, "Maximum number of ticks to simulate")
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

	inst, err := scenario.Build(doc, scenario.BuildOptions{
		Client:      client,
		Model:       model,
		Start:       *startID,
		Player:      *playerID,
		Interactive: true,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error building scenario instance: %v\n", err)
		os.Exit(1)
	}

	httpServer := sim.NewHTTPServer(inst.World, inst.HumanCtrl, *addr)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Capture Ctrl+C / SIGINT / SIGTERM for graceful exit
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\nReceived shutdown signal. Exiting...")
		httpServer.SetGameOver(errors.New("interrupted by user"))
		cancel()
	}()

	if err := httpServer.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start HTTP server: %v\n", err)
		os.Exit(1)
	}

	listenAddr := httpServer.Addr()
	if strings.HasPrefix(listenAddr, ":") {
		listenAddr = "http://127.0.0.1" + listenAddr
	} else if !strings.HasPrefix(listenAddr, "http://") {
		listenAddr = "http://" + listenAddr
	}

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

	fmt.Println("==================================================")
	fmt.Println("   OMNIA HTTP INTERACTIVE PLAY SERVER             ")
	fmt.Println("==================================================")
	fmt.Printf("Scenario:  %s\n", inst.Doc.Meta.Title)
	fmt.Printf("Setting:   %s\n", playerLocName)
	fmt.Printf("Player:    %s (Controlled via HTTP API)\n", inst.Player)
	fmt.Printf("Co-Stars:  %s\n", strings.Join(coStarNames, ", "))
	fmt.Printf("URL:       %s\n", listenAddr)
	fmt.Println("--------------------------------------------------")
	fmt.Println("API Endpoints:")
	fmt.Println("  GET  /turn         - Await / inspect current turn (JSON)")
	fmt.Println("  GET  /turn/text    - Current turn as formatted plain text")
	fmt.Println("  POST /action       - Submit action (JSON {\"action\":\"...\"} or raw text)")
	fmt.Println("  GET  /status       - View simulation state and tick")
	fmt.Println("  GET  /events       - View simulation event log")
	fmt.Println("  POST /quit         - Terminate simulation cleanly")
	fmt.Println("--------------------------------------------------")
	fmt.Println("Curl Examples:")
	fmt.Printf("  curl -s %s/turn | jq .\n", listenAddr)
	fmt.Printf("  curl -s %s/turn/text\n", listenAddr)
	fmt.Printf("  curl -s -X POST %s/action -d '{\"action\": \"Notes are good.\"}'\n", listenAddr)
	fmt.Printf("  curl -s -X POST %s/action -d \"Notes are good.\"\n", listenAddr)
	fmt.Println("==================================================")

	// Run the simulation loop
	simCtx, simCancel := context.WithTimeout(ctx, 30*time.Minute)
	defer simCancel()

	_, simErr := inst.World.Run(simCtx, *maxTicks, func(report sim.TickReport) {
		httpServer.OnTickReport(report)

		targetsInfo := ""
		if len(report.Reaction.Action.Targets) > 0 {
			targetsInfo = fmt.Sprintf(" -> [%s]", strings.Join(report.Reaction.Action.Targets, ", "))
		}

		if report.ActorID == inst.Player {
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
			fmt.Printf("\n--- Private Cognition ---\n%s\n", report.Reaction.Cognition)
		}

		fmt.Printf("\n--- Manifest Turn ---\nType:   %s%s\nAction: %s\n",
			report.Reaction.Action.Type, targetsInfo, report.Reaction.Action.Action)

		if report.SceneRuling != nil {
			fmt.Printf("\n--- Scene Arbiter Ruling (%s) ---\nResolution:   %s\n",
				report.SceneRuling.SceneID, report.SceneRuling.Resolution)
		}

		fmt.Printf("\nTurn Metrics: %v | %d tokens\n",
			report.Reaction.Duration.Round(time.Millisecond), report.Reaction.Usage.TotalTokens)
		fmt.Println("--------------------------------------------------")
	})

	if simErr != nil && !errors.Is(simErr, sim.ErrQuit) {
		fmt.Printf("Simulation ended with error: %v\n", simErr)
	}
}
