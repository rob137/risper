package main

import (
	"fmt"
	"os"

	"github.com/rob137/risper/benchmark"
	"github.com/rob137/risper/cli"
	"github.com/rob137/risper/diagnose"
	"github.com/rob137/risper/history"
	"github.com/rob137/risper/models"
	"github.com/rob137/risper/open"
	"github.com/rob137/risper/retranscribe"
	"github.com/rob137/risper/toggle"
)

func main() {
	os.Exit(Main(os.Args[1:]))
}

func Main(args []string) int {
	if len(args) == 0 {
		return cli.Main(nil)
	}
	if args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		printUsage()
		return 0
	}
	switch args[0] {
	case "start", "kill", "stop", "restart", "status":
		return cli.Main(args)
	case "toggle":
		return toggle.Main(args[1:])
	case "history":
		return history.Main(args[1:])
	case "open":
		return open.Main(args[1:])
	case "retranscribe":
		return retranscribe.Main(args[1:])
	case "models":
		return models.Main(args[1:])
	case "benchmark":
		return benchmark.Main(args[1:])
	case "diagnose":
		return diagnose.Main(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "risper: unknown command %q\n", args[0])
		printUsageTo(os.Stderr)
		return 2
	}
}

func printUsage() { printUsageTo(os.Stdout) }

func printUsageTo(output *os.File) {
	fmt.Fprintln(output, "Usage: risper <command> [options]")
	fmt.Fprintln(output)
	fmt.Fprintln(output, "Service: start (default), kill, stop, restart, status")
	fmt.Fprintln(output, "Dictation: toggle")
	fmt.Fprintln(output, "Sessions: history, open, retranscribe")
	fmt.Fprintln(output, "Models and diagnostics: models, benchmark, diagnose")
}
