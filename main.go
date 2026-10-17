package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"omnia.adityagupta.dev/pkg/scenario"
	"omnia.adityagupta.dev/pkg/sim"
)

func main() {
	playMode := flag.Bool("play", true, "Play interactively as Zack via terminal (defaults to true)")
	httpAddr := flag.String("http", "", "HTTP listen address for interactive play mode (e.g. :8080 or 127.0.0.1:8080)")
	maxTicks := flag.Int("ticks", 4, "Maximum number of ticks to simulate")
	flag.Parse()

	isInteractive := *playMode || *httpAddr != ""
	sc, err := scenario.Setup(isInteractive)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing scenario: %v\n", err)
		os.Exit(1)
	}

	var httpServer *sim.HTTPServer
	if *httpAddr != "" {
		httpServer = sim.NewHTTPServer(sc.World, sc.HumanCtrl, *httpAddr)
		if err := httpServer.Start(context.Background()); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to start HTTP server: %v\n", err)
			os.Exit(1)
		}
		listenAddr := httpServer.Addr()
		if strings.HasPrefix(listenAddr, ":") {
			listenAddr = "http://127.0.0.1" + listenAddr
		}
		fmt.Printf("\n[HTTP Server Online] Endpoints active at %s (/turn, /action, /status, /events)\n", listenAddr)
	} else if *playMode {
		// Terminal TTY Play Mode: scan standard input
		go func() {
			scanner := bufio.NewScanner(os.Stdin)
			for turn := range sc.HumanCtrl.Turns() {
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
					sc.HumanCtrl.Quit()
					return
				}
				input := strings.TrimSpace(scanner.Text())
				if input == "quit" || input == "exit" {
					sc.HumanCtrl.Quit()
					return
				}
				sc.HumanCtrl.Reply(input)
			}
		}()
	}

	// 5. Verification & Opening Scene Setup
	fmt.Println("==================================================")
	fmt.Println("   SCENARIO: ACCIDENTAL ARRANGEMENT (SLICE OF LIFE) ")
	fmt.Println("==================================================")
	fmt.Printf("Setting:   %s\n", sc.LivingRoom.Name)
	fmt.Printf("Player:    %s\n", sc.ZackMind.Profile.Name)
	fmt.Printf("Co-Stars:  %s, %s\n", sc.NellyMind.Profile.Name, sc.MiaMind.Profile.Name)
	fmt.Printf("Clock:     %s\n", sc.World.Clock)

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
		fmt.Println("       INTERACTIVE MODE: YOU ARE PLAYING ZACK     ")
	} else {
		fmt.Println("       AUTONOMOUS DEMONSTRATION MODE              ")
	}
	fmt.Println("==================================================")

	_, simErr := sc.World.Run(ctx, *maxTicks, func(report sim.TickReport) {
		if httpServer != nil {
			httpServer.OnTickReport(report)
		}

		targetsInfo := ""
		if len(report.Reaction.Action.Targets) > 0 {
			targetsInfo = fmt.Sprintf(" -> [%s]", strings.Join(report.Reaction.Action.Targets, ", "))
		}

		if isInteractive && report.ActorID == "zack" {
			fmt.Printf("\n[Your Action] (%s%s): %s\n", report.Reaction.Action.Type, targetsInfo, report.Reaction.Action.Action)
			if report.SceneRuling != nil {
				fmt.Printf("[Scene Resolution (%s)]: %s\n", report.SceneRuling.SceneID, report.SceneRuling.Resolution)
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

		if len(report.Thoughts) > 0 {
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
	for i, ev := range sc.World.GetEventLog() {
		fmt.Printf("[%d] Tick %d | Kind: %-7s | Source: %-15s | Loc: %s\n    %s\n",
			i+1, ev.Tick, ev.Kind, ev.SourceName, ev.LocationID, ev.Description)
	}

	// 8. Final Thought Graphs
	fmt.Println("\n==================================================")
	fmt.Println("           NELLY'S EVOLVED THOUGHT GRAPH          ")
	fmt.Println("==================================================")
	for _, th := range sc.NellyMind.ThoughtsSnapshot() {
		linksStr := ""
		if len(th.Links) > 0 {
			linksStr = fmt.Sprintf(" [links: %s]", strings.Join(th.Links, ", "))
		}
		fmt.Printf("- [%s:%-9s | salience: %.2f] %s%s\n", th.ID, th.Kind, th.Salience, th.Content, linksStr)
	}
}
