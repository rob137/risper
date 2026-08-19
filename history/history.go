// Package history implements the terminal history and session actions.
package history

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rob137/risper/config"
	"github.com/rob137/risper/desktop"
	"github.com/rob137/risper/internal/args"
	"github.com/rob137/risper/retranscribe"
	"github.com/rob137/risper/session"
	"github.com/rob137/risper/sessionactions"
)

func Main(argv []string) int {
	parser := flag.NewFlagSet("risper history", flag.ContinueOnError)
	parser.SetOutput(os.Stderr)
	limit := parser.Int("limit", 20, "number of recent sessions to show")
	openID := parser.String("open", "", "open a session folder by id")
	playID := parser.String("play", "", "open/play a session audio file by id")
	copyID := parser.String("copy", "", "copy a session transcript by id")
	retranscribeID := parser.String("retranscribe", "", "retranscribe a session by id")
	deleteID := parser.String("delete", "", "move a session to trash by id")
	prune := parser.Bool("prune-audio", false, "delete audio past audio_retention now")
	if err := parser.Parse(args.Reorder(argv, map[string]bool{
		"-limit": true, "--limit": true, "-open": true, "--open": true,
		"-play": true, "--play": true, "-copy": true, "--copy": true,
		"-retranscribe": true, "--retranscribe": true, "-delete": true, "--delete": true,
	})); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if parser.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "risper history: unexpected positional argument")
		return 2
	}
	if *limit < 0 {
		fmt.Fprintln(os.Stderr, "risper history: --limit must not be negative")
		return 2
	}
	cfg, err := config.Load()
	if err != nil {
		return reportError(err)
	}
	if *prune {
		if cfg.AudioRetentionSeconds == nil {
			fmt.Fprintln(os.Stderr, "audio_retention is 'never'; nothing to prune.")
			return 1
		}
		count, err := session.PruneExpiredAudio(cfg)
		if err != nil {
			return reportError(err)
		}
		fmt.Printf("Pruned audio from %d session(s).\n", count)
		return 0
	}
	sessions, err := session.All(cfg)
	if err != nil {
		return reportError(err)
	}
	if *openID != "" {
		return runAction(sessions, *openID, false, func(metadata *session.Metadata) (bool, string) {
			return sessionactions.OpenSession(metadata)
		})
	}
	if *playID != "" {
		return runAction(sessions, *playID, false, func(metadata *session.Metadata) (bool, string) {
			return sessionactions.PlayAudio(metadata)
		})
	}
	if *copyID != "" {
		return runAction(sessions, *copyID, true, func(metadata *session.Metadata) (bool, string) {
			return sessionactions.CopyTranscript(metadata)
		})
	}
	if *retranscribeID != "" {
		return retranscribe.Main([]string{*retranscribeID})
	}
	if *deleteID != "" {
		return deleteSession(sessions, *deleteID)
	}
	return printTable(sessions, *limit)
}

func printTable(sessions []*session.Metadata, limit int) int {
	if len(sessions) == 0 {
		fmt.Println("No Risper sessions yet.")
		return 0
	}
	if limit < len(sessions) {
		sessions = sessions[:limit]
	}
	if len(sessions) == 0 {
		fmt.Println("No Risper sessions yet.")
		return 0
	}
	fmt.Printf("%-22s %-14s %7s  preview\n", "session", "status", "dur")
	for _, metadata := range sessions {
		duration := ""
		if metadata.DurationSeconds != nil {
			duration = fmt.Sprintf("%gs", *metadata.DurationSeconds)
		}
		fmt.Printf("%-22s %-14s %7s  %s\n", metadata.SessionID, metadata.Status, duration, sessionactions.TranscriptPreview(metadata, 76))
	}
	return 0
}

func runAction(sessions []*session.Metadata, id string, printMessage bool, action func(*session.Metadata) (bool, string)) int {
	metadata := findByID(sessions, id)
	if metadata == nil {
		fmt.Fprintln(os.Stderr, "No such session: "+id)
		return 1
	}
	ok, message := action(metadata)
	if !ok {
		fmt.Fprintln(os.Stderr, message)
		return 1
	}
	if printMessage && message != "" {
		fmt.Println(message)
	}
	return 0
}

func deleteSession(sessions []*session.Metadata, id string) int {
	metadata := findByID(sessions, id)
	if metadata == nil {
		fmt.Fprintln(os.Stderr, "No such session: "+id)
		return 1
	}
	dir := session.SessionDir(metadata)
	fmt.Fprintf(os.Stdout, "Move %s to trash? Type DELETE to confirm: ", dir)
	answer, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && len(answer) == 0 {
		fmt.Fprintln(os.Stderr, "Could not read confirmation:", err)
		return 1
	}
	if strings.TrimSpace(answer) != "DELETE" {
		fmt.Println("Cancelled.")
		return 1
	}
	ok, message := desktop.TrashPath(dir)
	if !ok {
		if removeErr := os.RemoveAll(dir); removeErr != nil {
			fmt.Fprintln(os.Stderr, message)
			return 1
		}
		message = "removed permanently: " + filepath.Clean(dir)
	}
	fmt.Println(message)
	fmt.Println("Deleted " + metadata.SessionID)
	return 0
}

func findByID(sessions []*session.Metadata, id string) *session.Metadata {
	for _, metadata := range sessions {
		if metadata.SessionID == id {
			return metadata
		}
	}
	return nil
}

func reportError(err error) int {
	fmt.Fprintln(os.Stderr, "risper history:", err)
	return 1
}
