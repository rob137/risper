package desktop

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/rob137/risper/config"
)

const sendSoundFilter = "[1:a]rubberband=pitch=1.4983,adelay=170|170[b];[0:a][b]amix=inputs=2:normalize=0,alimiter=limit=0.95"

var soundExtensions = []string{".oga", ".ogg", ".wav", ".au", ".flac"}

// generatedSuccessSend returns a local, theme-derived copy of the success
// sound. It deliberately returns false for every lookup or generation failure:
// the event sound is the reliable fallback and a sound must never affect the
// toggle result.
func generatedSuccessSend(cfg config.Config, event string) (string, bool) {
	if event != "complete" || cfg.DataDir == "" || !config.CommandExists("ffmpeg") {
		return "", false
	}
	source, ok := resolveSoundFile(event)
	if !ok {
		return "", false
	}
	info, err := os.Stat(source)
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		return "", false
	}

	hash := sha256.New()
	_, _ = hash.Write([]byte(source))
	_, _ = hash.Write([]byte{'\x00'})
	_, _ = hash.Write([]byte(strconv.FormatInt(info.Size(), 10)))
	_, _ = hash.Write([]byte{'\x00'})
	_, _ = hash.Write([]byte(strconv.FormatInt(info.ModTime().UnixNano(), 10)))
	key := hex.EncodeToString(hash.Sum(nil))[:16]
	path := filepath.Join(cfg.DataDir, "success-send-"+key+".wav")
	if usableGeneratedSound(path) {
		return path, true
	}
	if !ffmpegHasFilter("rubberband") {
		return "", false
	}
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return "", false
	}

	temporary, err := os.CreateTemp(cfg.DataDir, ".success-send-*.wav")
	if err != nil {
		return "", false
	}
	temporaryPath := temporary.Name()
	if err := temporary.Close(); err != nil {
		_ = os.Remove(temporaryPath)
		return "", false
	}
	defer os.Remove(temporaryPath)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	args := []string{
		"-y", "-hide_banner", "-loglevel", "error",
		"-i", source, "-i", source,
		"-filter_complex", sendSoundFilter,
		"-c:a", "pcm_s16le", temporaryPath,
	}
	if err := exec.CommandContext(ctx, "ffmpeg", args...).Run(); err != nil {
		return "", false
	}
	if !usableGeneratedSound(temporaryPath) {
		return "", false
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		if usableGeneratedSound(path) {
			return path, true
		}
		return "", false
	}
	return path, true
}

func usableGeneratedSound(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Size() > 44
}

func ffmpegHasFilter(filter string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-filters").CombinedOutput()
	return err == nil && strings.Contains(string(output), filter)
}

func resolveSoundFile(event string) (string, bool) {
	theme := activeSoundTheme()
	themes := []string{theme}
	if theme != "freedesktop" {
		themes = append(themes, "freedesktop")
	}
	seen := make(map[string]bool, len(themes))
	for _, candidate := range themes {
		if path, ok := findSoundInTheme(candidate, event, seen); ok {
			return path, true
		}
	}
	return "", false
}

func activeSoundTheme() string {
	if !config.CommandExists("gsettings") {
		return "freedesktop"
	}
	output, err := exec.Command("gsettings", "get", "org.gnome.desktop.sound", "theme-name").Output()
	if err != nil {
		return "freedesktop"
	}
	theme := strings.Trim(strings.TrimSpace(string(output)), "'\"")
	if theme == "" || theme == "." || theme == ".." || strings.ContainsAny(theme, `/\\`) {
		return "freedesktop"
	}
	return theme
}

func findSoundInTheme(theme, event string, seen map[string]bool) (string, bool) {
	if seen[theme] {
		return "", false
	}
	seen[theme] = true
	directories, inherits, exists := soundThemeInfo(theme)
	if !exists {
		return "", false
	}
	for _, root := range soundDataDirs() {
		themeRoot := filepath.Join(root, "sounds", theme)
		if _, err := os.Stat(themeRoot); err != nil {
			continue
		}
		for _, directory := range directories {
			for _, extension := range soundExtensions {
				path := filepath.Join(themeRoot, directory, event+extension)
				if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
					return path, true
				}
			}
		}
	}
	for _, inherited := range inherits {
		if path, ok := findSoundInTheme(inherited, event, seen); ok {
			return path, true
		}
	}
	return "", false
}

func soundThemeInfo(theme string) ([]string, []string, bool) {
	for _, root := range soundDataDirs() {
		themeRoot := filepath.Join(root, "sounds", theme)
		info, err := os.Stat(themeRoot)
		if err != nil || !info.IsDir() {
			continue
		}
		directories, inherits := parseSoundThemeIndex(filepath.Join(themeRoot, "index.theme"))
		if len(directories) == 0 {
			directories = []string{"stereo"}
		}
		return directories, inherits, true
	}
	return nil, nil, false
}

func parseSoundThemeIndex(path string) ([]string, []string) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil
	}
	defer file.Close()

	section := ""
	var directories, inherits []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]"))
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || section != "Sound Theme" {
			continue
		}
		switch strings.TrimSpace(key) {
		case "Directories":
			directories = splitThemeList(value)
		case "Inherits":
			inherits = splitThemeList(value)
		}
	}
	return directories, inherits
}

func splitThemeList(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" && part != "." && part != ".." && !strings.ContainsAny(part, `/\\`) {
			result = append(result, part)
		}
	}
	return result
}

func soundDataDirs() []string {
	var roots []string
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			dataHome = filepath.Join(home, ".local", "share")
		}
	}
	if dataHome != "" {
		roots = append(roots, dataHome)
	}
	dataDirs := os.Getenv("XDG_DATA_DIRS")
	if dataDirs == "" {
		dataDirs = "/usr/local/share:/usr/share"
	}
	roots = append(roots, strings.Split(dataDirs, string(os.PathListSeparator))...)
	return uniqueNonEmptyPaths(roots)
}

func uniqueNonEmptyPaths(paths []string) []string {
	seen := make(map[string]bool, len(paths))
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		path = filepath.Clean(path)
		if path == "." || path == "" || seen[path] {
			continue
		}
		seen[path] = true
		result = append(result, path)
	}
	return result
}
