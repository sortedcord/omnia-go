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

	"omnia.adityagupta.dev/pkg/scenario"
	"omnia.adityagupta.dev/pkg/sim"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP server listen address (e.g. :8080 or 127.0.0.1:8080)")
	maxTicks := flag.Int("ticks", 10, "Maximum number of ticks to simulate")
	flag.Parse()

	sc, err := scenario.Setup(true)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error setting up scenario: %v\n", err)
		os.Exit(1)
	}

	httpServer := sim.NewHTTPServer(sc.World, sc.HumanCtrl, *addr)

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

	fmt.Println("==================================================")
	fmt.Println("   OMNIA HTTP INTERACTIVE PLAY SERVER             ")
	fmt.Println("==================================================")
	fmt.Printf("Scenario:  Accidental Arrangement\n")
	fmt.Printf("Player:    Zack (Controlled via HTTP API)\n")
	fmt.Printf("Co-Stars:  Nelly, Mia\n")
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

	_, simErr := sc.World.Run(simCtx, *maxTicks, func(report sim.TickReport) {
		httpServer.OnTickReport(report)

		targetsInfo := ""
		if len(report.Reaction.Action.Targets) > 0 {
			targetsInfo = fmt.Sprintf(" -> [%s]", strings.Join(report.Reaction.Action.Targets, ", "))
		}

		if report.ActorID == "zack" {
			fmt.Printf("\n[Player Action (Zack)] (%s%s): %s\n",
				report.Reaction.Action.Type, targetsInfo, report.Reaction.Action.Action)
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

	httpServer.SetGameOver(simErr)

	if simErr != nil && !errors.Is(simErr, sim.ErrQuit) && !errors.Is(simErr, context.Canceled) {
		fmt.Printf("Simulation ended with error: %v\n", simErr)
	} else {
		fmt.Println("\nSimulation run complete.")
	}

	// Print Event Log summary
	fmt.Println("\n==================================================")
	fmt.Println("               SIMULATION EVENT LOG               ")
	fmt.Println("==================================================")
	for i, ev := range sc.World.GetEventLog() {
		fmt.Printf("[%d] Tick %d | Kind: %-7s | Source: %-15s | Loc: %s\n    %s\n",
			i+1, ev.Tick, ev.Kind, ev.SourceName, ev.LocationID, ev.Description)
	}
}
