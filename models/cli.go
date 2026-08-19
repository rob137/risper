package models

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/rob137/risper/config"
	"github.com/rob137/risper/internal/args"
)

// Main implements the model profile command formerly exposed as
// risper-models. The profile registry itself remains in this package.
func Main(argv []string) int {
	if len(argv) == 0 {
		return list()
	}
	action := argv[0]
	if strings.HasPrefix(action, "-") {
		if action == "-h" || action == "--help" {
			fmt.Fprintln(os.Stderr, "Usage: risper models [list|current|select|add-external]")
			return 0
		}
		fmt.Fprintln(os.Stderr, "risper models: action is required")
		return 2
	}
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "risper models:", err)
		return 1
	}
	switch action {
	case "list":
		if len(argv) != 1 {
			return unexpectedArgs("list")
		}
		return listWithConfig(cfg)
	case "current":
		if len(argv) != 1 {
			return unexpectedArgs("current")
		}
		profile, err := Active(cfg)
		if err != nil {
			fmt.Fprintln(os.Stderr, "risper models:", err)
			return 1
		}
		fmt.Printf("%s: %s %s (%s)\n", profile.ID, profile.Engine, profile.Model, profile.Language)
		return 0
	case "select":
		if len(argv) != 2 {
			return unexpectedArgs("select PROFILE_ID")
		}
		profiles, err := Load(cfg)
		if err != nil {
			fmt.Fprintln(os.Stderr, "risper models:", err)
			return 1
		}
		if _, ok := profiles[argv[1]]; !ok {
			fmt.Fprintln(os.Stderr, "No such profile: "+argv[1])
			return 1
		}
		if err := Select(argv[1]); err != nil {
			fmt.Fprintln(os.Stderr, "risper models:", err)
			return 1
		}
		fmt.Println("Selected " + argv[1])
		return 0
	case "add-external":
		return addExternal(cfg, argv[1:])
	default:
		fmt.Fprintln(os.Stderr, "risper models: invalid action "+action)
		return 2
	}
}

func list() int {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "risper models:", err)
		return 1
	}
	return listWithConfig(cfg)
}

func listWithConfig(cfg config.Config) int {
	profiles, err := Load(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "risper models:", err)
		return 1
	}
	if len(profiles) == 0 {
		fmt.Printf("No profiles configured. Edit %s.\n", cfg.ModelsPath)
		return 0
	}
	active, err := Active(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "risper models:", err)
		return 1
	}
	ids := make([]string, 0, len(profiles))
	for id := range profiles {
		ids = append(ids, id)
	}
	// Load uses the same sorted order as Active's fallback, and keeping the
	// sorting local avoids making output depend on Go map iteration.
	sortStrings(ids)
	fmt.Printf("%-24s %-14s %-16s language\n", "id", "engine", "model")
	for _, id := range ids {
		profile := profiles[id]
		marker := " "
		if profile.ID == active.ID {
			marker = "*"
		}
		fmt.Printf("%s %-22s %-14s %-16s %s\n", marker, id, profile.Engine, profile.Model, profile.Language)
	}
	return 0
}

func addExternal(cfg config.Config, argv []string) int {
	parser := flag.NewFlagSet("risper models add-external", flag.ContinueOnError)
	parser.SetOutput(os.Stderr)
	engine := parser.String("engine", "", "local engine name")
	model := parser.String("model", "", "local model name")
	command := parser.String("command", "", "local transcription command")
	language := parser.String("language", "en", "language")
	selectProfile := parser.Bool("select", false, "select the profile after adding it")
	if err := parser.Parse(args.Reorder(argv, map[string]bool{
		"-engine": true, "--engine": true, "-model": true, "--model": true,
		"-command": true, "--command": true, "-language": true, "--language": true,
	})); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if parser.NArg() != 1 || *engine == "" || *model == "" || *command == "" {
		fmt.Fprintln(os.Stderr, "Usage: risper models add-external PROFILE_ID --engine ENGINE --model MODEL --command COMMAND [--language LANG] [--select]")
		return 2
	}
	profile := Profile{ID: parser.Arg(0), Engine: *engine, Model: *model, Language: *language, Command: *command}
	if err := Write(cfg, profile, *selectProfile); err != nil {
		fmt.Fprintln(os.Stderr, "risper models:", err)
		return 1
	}
	fmt.Println("Added " + profile.ID)
	return 0
}

func unexpectedArgs(usage string) int {
	fmt.Fprintln(os.Stderr, "Usage: risper models "+usage)
	return 2
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}
