// Package benchmark measures the configured local transcription commands.
package benchmark

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/rob137/risper/config"
	"github.com/rob137/risper/internal/args"
	"github.com/rob137/risper/models"
	"github.com/rob137/risper/session"
	"github.com/rob137/risper/transcription"
)

type Result struct {
	Profile           string   `json:"profile"`
	Engine            string   `json:"engine"`
	Model             string   `json:"model"`
	ReturnCode        int      `json:"returncode"`
	ElapsedSeconds    float64  `json:"elapsed_seconds"`
	UserSeconds       float64  `json:"user_seconds"`
	SystemSeconds     float64  `json:"system_seconds"`
	CPUPercent        float64  `json:"cpu_percent"`
	MaxRSSMB          float64  `json:"max_rss_mb"`
	TranscriptChars   int      `json:"transcript_chars"`
	TranscriptPreview string   `json:"transcript_preview"`
	StderrTail        []string `json:"stderr_tail"`
	Repeat            int      `json:"repeat"`
	AudioPath         string   `json:"audio_path"`
}

type stringList []string

func (values *stringList) String() string { return strings.Join(*values, ",") }
func (values *stringList) Set(value string) error {
	*values = append(*values, value)
	return nil
}

func Main(argv []string) int {
	parser := flag.NewFlagSet("risper benchmark", flag.ContinueOnError)
	parser.SetOutput(os.Stderr)
	var profileIDs stringList
	parser.Var(&profileIDs, "profile", "profile id to benchmark; repeatable")
	repeat := parser.Int("repeat", 1, "number of times to run each profile")
	jsonOutput := parser.Bool("json", false, "emit JSON instead of a table")
	if err := parser.Parse(args.Reorder(argv, map[string]bool{
		"-profile": true, "--profile": true, "-repeat": true, "--repeat": true,
	})); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if parser.NArg() != 1 || *repeat <= 0 {
		fmt.Fprintln(os.Stderr, "Usage: risper benchmark SESSION_OR_AUDIO [--profile ID] [--repeat N] [--json]")
		return 2
	}
	cfg, err := config.Load()
	if err != nil {
		return reportError(err)
	}
	input := parser.Arg(0)
	metadata, err := session.Find(cfg, input)
	if err != nil {
		return reportError(err)
	}
	audioPath := input
	if metadata != nil {
		audioPath = metadata.AudioPath
	}
	audioPath, err = filepath.Abs(expandPath(audioPath))
	if err != nil {
		return reportError(err)
	}
	if _, err := os.Stat(audioPath); err != nil {
		fmt.Fprintln(os.Stderr, "Audio not found:", audioPath)
		return 1
	}
	profiles, err := models.Load(cfg)
	if err != nil {
		return reportError(err)
	}
	if len(profileIDs) == 0 {
		active, activeErr := models.Active(cfg)
		if activeErr != nil {
			return reportError(activeErr)
		}
		profileIDs = append(profileIDs, active.ID)
	}
	results := make([]Result, 0, len(profileIDs)*(*repeat))
	for _, profileID := range profileIDs {
		profile, ok := profiles[profileID]
		if !ok {
			fmt.Fprintln(os.Stderr, "No such profile: "+profileID)
			return 1
		}
		for index := 1; index <= *repeat; index++ {
			result, runErr := runProfile(profile, audioPath)
			if runErr != nil {
				return reportError(runErr)
			}
			result.Repeat = index
			result.AudioPath = audioPath
			results = append(results, result)
		}
	}
	if *jsonOutput {
		data, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			return reportError(err)
		}
		fmt.Println(string(data))
		return 0
	}
	fmt.Printf("%-24s %3s %8s %7s %8s %6s  preview\n", "profile", "rep", "wall", "cpu%", "rss_mb", "chars")
	for _, result := range results {
		fmt.Printf("%-24s %3d %8.3f %7.1f %8.1f %6d  %s\n",
			result.Profile, result.Repeat, result.ElapsedSeconds, result.CPUPercent,
			result.MaxRSSMB, result.TranscriptChars, result.TranscriptPreview)
	}
	return 0
}

func runProfile(profile models.Profile, audioPath string) (Result, error) {
	root, err := os.MkdirTemp("", "risper-bench-")
	if err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(root)
	rawPath := filepath.Join(root, "transcript.raw.txt")
	cleanPath := filepath.Join(root, "transcript.clean.txt")
	before := childUsage()
	started := time.Now()
	transcript, runErr := transcription.Transcribe(profile, audioPath, rawPath, cleanPath, nil)
	elapsed := time.Since(started).Seconds()
	after := childUsage()
	returnCode := 0
	stderrTail := []string{}
	if runErr != nil {
		returnCode = -1
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			returnCode = exitErr.ExitCode()
		}
		stderrTail = errorTail(runErr, 5)
	}
	userSeconds := duration(after.Utime) - duration(before.Utime)
	systemSeconds := duration(after.Stime) - duration(before.Stime)
	cpuPercent := 0.0
	if elapsed > 0 {
		cpuPercent = (userSeconds + systemSeconds) / elapsed * 100
	}
	return Result{
		Profile: profile.ID, Engine: profile.Engine, Model: profile.Model,
		ReturnCode: returnCode, ElapsedSeconds: round(elapsed, 3),
		UserSeconds: round(userSeconds, 3), SystemSeconds: round(systemSeconds, 3),
		CPUPercent: round(cpuPercent, 1), MaxRSSMB: round(float64(after.Maxrss)/1024, 1),
		TranscriptChars:   len([]rune(transcript)),
		TranscriptPreview: truncate(strings.Join(strings.Fields(transcript), " "), 100),
		StderrTail:        stderrTail,
	}, nil
}

func childUsage() syscall.Rusage {
	var usage syscall.Rusage
	_ = syscall.Getrusage(syscall.RUSAGE_CHILDREN, &usage)
	return usage
}

func duration(value syscall.Timeval) float64 {
	return float64(value.Sec) + float64(value.Usec)/1_000_000
}

func tailLines(text string, count int) []string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return []string{}
	}
	if len(lines) > count {
		lines = lines[len(lines)-count:]
	}
	return lines
}

func errorTail(err error, count int) []string {
	if err == nil {
		return []string{}
	}
	lines := tailLines(err.Error(), count)
	const maxLineRunes = 512
	for index, line := range lines {
		lines[index] = truncate(line, maxLineRunes)
	}
	return lines
}

func truncate(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit])
}

func round(value float64, places int) float64 {
	power := 1.0
	for index := 0; index < places; index++ {
		power *= 10
	}
	return float64(int(value*power+0.5)) / power
}

func expandPath(path string) string {
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

func reportError(err error) int {
	fmt.Fprintln(os.Stderr, "risper benchmark:", err)
	return 1
}

// Codex gpt-5.6-sol, xhigh, prompted by Robert Kirby
