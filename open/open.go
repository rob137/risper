// Package open implements the files-and-folders command.
package open

import (
	"flag"
	"fmt"
	"os"

	"github.com/rob137/risper/config"
	"github.com/rob137/risper/desktop"
	"github.com/rob137/risper/internal/args"
	"github.com/rob137/risper/session"
	"github.com/rob137/risper/sessionactions"
)

var targets = map[string]bool{
	"recordings": true, "last-session": true, "last-transcript": true,
	"last-audio": true, "play-last": true, "config": true, "copy-last": true,
}

func Main(argv []string) int {
	parser := flag.NewFlagSet("risper open", flag.ContinueOnError)
	parser.SetOutput(os.Stderr)
	if err := parser.Parse(args.Reorder(argv, nil)); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	target := "recordings"
	if parser.NArg() > 1 {
		fmt.Fprintln(os.Stderr, "risper open: unexpected positional arguments")
		return 2
	}
	if parser.NArg() == 1 {
		target = parser.Arg(0)
	}
	if !targets[target] {
		fmt.Fprintf(os.Stderr, "risper open: invalid target %q\n", target)
		return 2
	}
	cfg, err := config.Load()
	if err != nil {
		return reportError("risper open", err)
	}
	if target == "recordings" {
		return openPath(cfg.SessionsDir)
	}
	if target == "config" {
		return openPath(cfg.ConfigPath)
	}
	metadata, err := session.Last(cfg)
	if err != nil {
		return reportError("risper open", err)
	}
	if metadata == nil {
		fmt.Fprintln(os.Stderr, "No Risper sessions yet.")
		return 1
	}
	switch target {
	case "last-session":
		return openPath(session.SessionDir(metadata))
	case "last-audio", "play-last":
		return openPath(metadata.AudioPath)
	case "last-transcript":
		path := sessionactions.TranscriptPath(metadata)
		if path == "" {
			fmt.Fprintln(os.Stderr, "Last session has no transcript.")
			return 1
		}
		return openPath(path)
	case "copy-last":
		ok, message := sessionactions.CopyTranscript(metadata)
		fmt.Println(message)
		if ok {
			return 0
		}
		return 1
	default:
		return 2
	}
}

func openPath(path string) int {
	ok, message := desktop.OpenPath(path)
	if !ok {
		fmt.Fprintln(os.Stderr, message)
		return 1
	}
	return 0
}

func reportError(prefix string, err error) int {
	fmt.Fprintln(os.Stderr, prefix+":", err)
	return 1
}
