package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"omnia.adityagupta.dev/pkg/scenario"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "scenario":
		handleScenario(os.Args[2:])
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command %q\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Usage: omnia <command> [arguments]")
	fmt.Println("\nCommands:")
	fmt.Println("  scenario validate <file.omnia.json> [--start <id>] [--json]   Validate a scenario document")
}

func handleScenario(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: omnia scenario <validate> [args]")
		os.Exit(1)
	}

	switch args[0] {
	case "validate":
		validateCmd := flag.NewFlagSet("validate", flag.ExitOnError)
		startID := validateCmd.String("start", "", "Optional start ID to validate specifically")
		jsonOutput := validateCmd.Bool("json", false, "Output validation report as JSON")
		_ = validateCmd.Parse(args[1:])

		files := validateCmd.Args()
		if len(files) < 1 {
			fmt.Fprintln(os.Stderr, "Error: missing scenario file path")
			fmt.Fprintln(os.Stderr, "Usage: omnia scenario validate <file.omnia.json> [--start <id>] [--json]")
			os.Exit(1)
		}

		filePath := files[0]
		f, err := os.Open(filePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening file %q: %v\n", filePath, err)
			os.Exit(1)
		}
		defer f.Close()

		doc, err := scenario.Decode(f)
		if err != nil {
			if *jsonOutput {
				report := scenario.ValidationReport{
					Valid: false,
					Issues: []scenario.Issue{
						{Pointer: "/", Severity: scenario.SeverityError, Code: "DECODE_ERROR", Message: err.Error()},
					},
				}
				data, _ := json.MarshalIndent(report, "", "  ")
				fmt.Println(string(data))
			} else {
				fmt.Fprintf(os.Stderr, "Decode error in %s:\n  [ERROR] %v\n", filePath, err)
			}
			os.Exit(1)
		}

		report := scenario.Validate(doc, *startID)
		if *jsonOutput {
			data, _ := json.MarshalIndent(report, "", "  ")
			fmt.Println(string(data))
		} else {
			if len(report.Issues) == 0 {
				fmt.Printf("✓ %s is valid (scenario %s %s)\n", filePath, doc.Meta.ID, doc.Format.Version)
				if len(report.ActorsReachedOnTick1) > 0 {
					fmt.Printf("  Tick-1 active actors: %s\n", strings.Join(report.ActorsReachedOnTick1, ", "))
				}
			} else {
				fmt.Printf("Validation report for %s:\n", filePath)
				for _, iss := range report.Issues {
					fmt.Printf("  [%s] %s (%s): %s\n", iss.Severity, iss.Pointer, iss.Code, iss.Message)
				}
				if len(report.ActorsReachedOnTick1) > 0 {
					fmt.Printf("  Tick-1 active actors: %s\n", strings.Join(report.ActorsReachedOnTick1, ", "))
				}
				if report.Valid {
					fmt.Printf("✓ %s passed with warnings/notes\n", filePath)
				} else {
					fmt.Printf("✗ %s failed validation\n", filePath)
				}
			}
		}

		if !report.Valid {
			os.Exit(1)
		}

	default:
		fmt.Fprintf(os.Stderr, "Unknown scenario subcommand %q\n", args[0])
		os.Exit(1)
	}
}
