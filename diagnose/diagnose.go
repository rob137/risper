// Package diagnose prints environment and session diagnostics without
// including transcript contents.
package diagnose

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/rob137/risper/config"
	"github.com/rob137/risper/desktop"
	"github.com/rob137/risper/events"
	"github.com/rob137/risper/models"
	"github.com/rob137/risper/session"
	"github.com/rob137/risper/transcription"
)

func Main(argv []string) int {
	parser := flag.NewFlagSet("risper diagnose", flag.ContinueOnError)
	parser.SetOutput(os.Stderr)
	if err := parser.Parse(argv); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if parser.NArg() > 1 {
		fmt.Fprintln(os.Stderr, "risper diagnose: unexpected positional arguments")
		return 2
	}
	if parser.NArg() == 1 {
		return printSession(parser.Arg(0))
	}
	return printEnvironment()
}

func printSession(sessionID string) int {
	cfg, err := config.Load()
	if err != nil {
		return reportError(err)
	}
	metadata, err := session.Find(cfg, sessionID)
	if err != nil {
		return reportError(err)
	}
	if metadata == nil {
		fmt.Println("No such session: " + sessionID)
		return 1
	}
	root := session.SessionDir(metadata)
	audioSources := metadata.AudioSources
	if len(audioSources) == 0 {
		audioSources = []string{"mic"}
	}
	fmt.Println("Risper session diagnosis: " + metadata.SessionID)
	fmt.Println(strings.Repeat("=", 64))
	fmt.Printf("session_dir          %s\n", root)
	fmt.Printf("status               %s\n", metadata.Status)
	fmt.Printf("started_at           %s\n", metadata.StartedAt)
	fmt.Printf("ended_at             %s\n", valueOrBlank(metadata.EndedAt))
	fmt.Printf("duration_seconds     %v\n", valueOrBlank(metadata.DurationSeconds))
	fmt.Printf("session_type         %s\n", metadata.SessionType)
	fmt.Printf("audio_sources        %s\n", strings.Join(audioSources, ","))
	fmt.Printf("engine               %s\n", metadata.TranscriptionEngine)
	fmt.Printf("model                %s\n", metadata.Model)
	fmt.Printf("language             %s\n", metadata.Language)
	fmt.Printf("paste_attempted      %v\n", valueOrBlank(metadata.PasteAttempted))
	fmt.Printf("paste_succeeded      %v\n", valueOrBlank(metadata.PasteSucceeded))
	fmt.Printf("paste_helper_ok      %v\n", valueOrBlank(metadata.PasteHelperSucceeded))
	fmt.Printf("paste_confirmation   %s\n", metadata.PasteConfirmation)
	fmt.Printf("errors               %d\n", len(metadata.Errors))
	for _, item := range metadata.Errors {
		fmt.Println("  - " + item)
	}
	fmt.Println()
	fmt.Println("Files:")
	paths := []struct {
		label string
		path  string
	}{
		{"audio", metadata.AudioPath},
		{"raw transcript", metadata.TranscriptRawPath},
		{"clean transcript", metadata.TranscriptCleanPath},
		{"metadata", filepath.Join(root, session.MetadataFile)},
		{"events", events.Path(root)},
		{"status log", filepath.Join(root, session.StatusLogFile)},
		{"error log", filepath.Join(root, session.ErrorLogFile)},
		{"recorder log", filepath.Join(root, "pw-record.log")},
		{"recorder log sys", filepath.Join(root, "pw-record.system.log")},
		{"unmixed mic", sourcePath(metadata, "mic")},
		{"unmixed system", sourcePath(metadata, "system")},
		{"daemon log", cfg.LogPath},
	}
	for _, item := range paths {
		present, size := fileInfo(item.path)
		mark := "no"
		if present {
			mark = "yes"
		}
		fmt.Printf("  %-16s %-3s %9d  %s\n", item.label, mark, size, item.path)
	}
	fmt.Println()
	fmt.Println("Recent events:")
	recent, err := events.Read(root, 12)
	if err == nil {
		for _, item := range recent {
			timestamp, _ := item["timestamp"].(string)
			event, _ := item["event"].(string)
			detail := make(map[string]any)
			for key, value := range item {
				if key != "timestamp" && key != "event" {
					detail[key] = value
				}
			}
			data, _ := json.Marshal(detail)
			fmt.Printf("  %s %s %s\n", timestamp, event, string(data))
		}
	}
	printTail("Status log tail:", filepath.Join(root, session.StatusLogFile), 8)
	printTail("Error log tail:", filepath.Join(root, session.ErrorLogFile), 8)
	printTail("Daemon log tail:", cfg.LogPath, 12)
	return 0
}

func printEnvironment() int {
	cfg, err := config.Load()
	if err != nil {
		return reportError(err)
	}
	fmt.Println("Risper diagnosis")
	fmt.Println("=====================")
	fmt.Printf("Platform: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Println(run([]string{"lsb_release", "-a"}))
	fmt.Println()
	fmt.Printf("XDG_SESSION_TYPE=%s\n", os.Getenv("XDG_SESSION_TYPE"))
	fmt.Printf("XDG_CURRENT_DESKTOP=%s\n", os.Getenv("XDG_CURRENT_DESKTOP"))
	fmt.Printf("DESKTOP_SESSION=%s\n", os.Getenv("DESKTOP_SESSION"))
	fmt.Println()
	fmt.Println(run([]string{"gnome-shell", "--version"}))
	fmt.Println()
	fmt.Println("Commands:")
	for _, command := range desktop.DiagnosticCommands() {
		path, err := exec.LookPath(command)
		if err != nil {
			path = "-"
		}
		fmt.Printf("  %-18s %s\n", command, path)
	}
	fmt.Println()
	fmt.Println("Paste automation:")
	fmt.Println("  disabled           transcript is copied to the clipboard")
	fmt.Println()
	fmt.Printf("Go: %s\n", runtime.Version())
	fmt.Println()
	fmt.Println("Risper config:")
	fmt.Printf("  config              %s\n", cfg.ConfigPath)
	fmt.Printf("  sessions            %s\n", cfg.SessionsDir)
	fmt.Printf("  transcription       %s\n", cfg.TranscriptionEngine)
	profiles, err := models.Load(cfg)
	if err != nil {
		return reportError(err)
	}
	fmt.Printf("  models file         %s\n", cfg.ModelsPath)
	fmt.Printf("  model profiles      %d\n", len(profiles))
	if len(profiles) > 0 {
		profile, activeErr := models.Active(cfg)
		if activeErr == nil {
			fmt.Printf("  selected model      %s\n", profile.ID)
			fmt.Printf("  selected engine     %s\n", profile.Engine)
			fmt.Printf("  selected model name %s\n", profile.Model)
			binary := strings.Fields(profile.Command)
			commandPath := "missing"
			if len(binary) > 0 {
				if found, lookErr := exec.LookPath(binary[0]); lookErr == nil {
					commandPath = "yes (" + found + ")"
				} else if _, statErr := os.Stat(binary[0]); statErr == nil {
					commandPath = "yes (" + binary[0] + ")"
				}
			}
			fmt.Printf("  command binary      %s\n", commandPath)
		}
	}
	fmt.Printf("  paste mode          %s\n", cfg.PasteMode)
	if cfg.DoubleAltEnabled {
		fmt.Println("  double Alt          enabled")
	} else {
		fmt.Println("  double Alt          disabled")
	}
	fmt.Printf("  double Alt window   %d ms\n", cfg.DoubleAltWindowMS)
	if current, currentErr := transcription.Current(cfg); currentErr == nil && current != nil {
		worker := "<nil>"
		if current.WorkerPID != nil {
			worker = fmt.Sprint(*current.WorkerPID)
		}
		fmt.Printf("  active transcription %s worker=%s\n", current.SessionDir, worker)
	}
	return 0
}

func run(command []string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, command[0], command[1:]...).CombinedOutput()
	if err != nil {
		return "unavailable: " + err.Error()
	}
	text := strings.TrimSpace(string(output))
	if text == "" {
		return "unavailable"
	}
	return text
}

func sourcePath(metadata *session.Metadata, source string) string {
	if path := metadata.AudioSourcePaths[source]; path != "" {
		return path
	}
	return filepath.Join(session.SessionDir(metadata), "audio."+source+".wav")
}

func fileInfo(path string) (bool, int64) {
	info, err := os.Stat(path)
	if err != nil {
		return false, 0
	}
	return true, info.Size()
}

func printTail(label, path string, lines int) {
	fmt.Println()
	fmt.Println(label)
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	values := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(values) > lines {
		values = values[len(values)-lines:]
	}
	for _, value := range values {
		if value != "" {
			fmt.Println("  " + value)
		}
	}
}

func valueOrBlank(value any) any {
	switch typed := value.(type) {
	case nil:
		return ""
	case *string:
		if typed == nil {
			return ""
		}
		return *typed
	case *float64:
		if typed == nil {
			return ""
		}
		return *typed
	case *bool:
		if typed == nil {
			return ""
		}
		return *typed
	default:
		return value
	}
}

func reportError(err error) int {
	fmt.Fprintln(os.Stderr, "risper diagnose:", err)
	return 1
}
