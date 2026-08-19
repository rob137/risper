package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/rob137/risper/config"
	"github.com/rob137/risper/events"
	"github.com/rob137/risper/internal/files"
)

const recoveryMessage = "Recovered incomplete recording after startup; audio may be partial."

const (
	AudioFile           = "audio.wav"
	MetadataFile        = "metadata.json"
	EventsFile          = "events.jsonl"
	StatusLogFile       = "status.log"
	ErrorLogFile        = "error.log"
	TranscriptRawFile   = "transcript.raw.txt"
	TranscriptCleanFile = "transcript.clean.txt"
)

type Metadata struct {
	AudioPath            string                     `json:"audio_path"`
	AudioSources         []string                   `json:"audio_sources,omitempty"`
	AudioSourcePaths     map[string]string          `json:"audio_source_paths,omitempty"`
	AudioPrunedAt        *string                    `json:"audio_pruned_at,omitempty"`
	DurationSeconds      *float64                   `json:"duration_seconds"`
	EndedAt              *string                    `json:"ended_at"`
	Errors               []string                   `json:"errors"`
	Language             string                     `json:"language"`
	Model                string                     `json:"model"`
	PasteAttempted       *bool                      `json:"paste_attempted,omitempty"`
	PasteConfirmation    string                     `json:"paste_confirmation,omitempty"`
	PasteHelperSucceeded *bool                      `json:"paste_helper_succeeded,omitempty"`
	PasteSucceeded       *bool                      `json:"paste_succeeded,omitempty"`
	SessionID            string                     `json:"session_id"`
	SessionType          string                     `json:"session_type"`
	StartedAt            string                     `json:"started_at"`
	Status               string                     `json:"status"`
	TargetApp            *string                    `json:"target_app"`
	TranscriptCleanPath  string                     `json:"transcript_clean_path"`
	TranscriptRawPath    string                     `json:"transcript_raw_path"`
	TranscriptionEngine  string                     `json:"transcription_engine"`
	Extra                map[string]json.RawMessage `json:"-"`
}

func boolPtr(value bool) *bool { return &value }

var knownMetadataKeys = map[string]struct{}{
	"audio_path": {}, "audio_sources": {}, "audio_source_paths": {}, "audio_pruned_at": {}, "duration_seconds": {}, "ended_at": {},
	"errors": {}, "language": {}, "model": {}, "paste_attempted": {}, "paste_confirmation": {},
	"paste_helper_succeeded": {}, "paste_succeeded": {}, "session_id": {}, "session_type": {},
	"started_at": {}, "status": {}, "target_app": {}, "transcript_clean_path": {}, "transcript_raw_path": {},
	"transcription_engine": {},
}

func (metadata *Metadata) UnmarshalJSON(data []byte) error {
	type plain Metadata
	var value plain
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for key := range knownMetadataKeys {
		delete(fields, key)
	}
	value.Extra = fields
	*metadata = Metadata(value)
	return nil
}

func (metadata Metadata) MarshalJSON() ([]byte, error) {
	type plain Metadata
	data, err := json.Marshal(plain(metadata))
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	for key, value := range metadata.Extra {
		if _, known := knownMetadataKeys[key]; !known {
			fields[key] = value
		}
	}
	return json.Marshal(fields)
}

func nowISO(now time.Time) string { return now.Format(time.RFC3339) }

func SessionDir(metadata *Metadata) string  { return filepath.Dir(metadata.AudioPath) }
func MetadataPath(sessionDir string) string { return filepath.Join(sessionDir, MetadataFile) }
func EventsPath(metadata *Metadata) string  { return events.Path(SessionDir(metadata)) }

func currentSessionType() string {
	if value := strings.ToLower(strings.TrimSpace(os.Getenv("XDG_SESSION_TYPE"))); value != "" {
		return value
	}
	return "wayland"
}

func sessionID(now time.Time) string { return now.In(time.Local).Format("2006-01-02_15-04-05") }

func Create(cfg config.Config) (*Metadata, error) { return CreateAt(cfg, time.Now()) }

func CreateAt(cfg config.Config, now time.Time) (*Metadata, error) {
	id := sessionID(now)
	if err := os.MkdirAll(cfg.SessionsDir, 0o755); err != nil {
		return nil, err
	}
	dir := filepath.Join(cfg.SessionsDir, id)
	for counter := 2; ; counter++ {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			break
		} else if err != nil {
			return nil, err
		}
		dir = filepath.Join(cfg.SessionsDir, fmt.Sprintf("%s-%d", id, counter))
	}
	if err := os.Mkdir(dir, 0o755); err != nil {
		return nil, err
	}
	started := nowISO(now)
	metadata := &Metadata{
		AudioPath:            filepath.Join(dir, AudioFile),
		AudioSourcePaths:     map[string]string{},
		Errors:               []string{},
		Language:             cfg.Language,
		Model:                cfg.Model,
		PasteAttempted:       boolPtr(false),
		PasteConfirmation:    "not_attempted",
		PasteHelperSucceeded: boolPtr(false),
		PasteSucceeded:       boolPtr(false),
		SessionID:            filepath.Base(dir),
		SessionType:          currentSessionType(),
		StartedAt:            started,
		Status:               "recording",
		TranscriptCleanPath:  filepath.Join(dir, TranscriptCleanFile),
		TranscriptRawPath:    filepath.Join(dir, TranscriptRawFile),
		TranscriptionEngine:  cfg.TranscriptionEngine,
	}
	if err := SaveMetadata(metadata); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, StatusLogFile), []byte(started+" session created\n"), 0o644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, ErrorLogFile), nil, 0o644); err != nil {
		return nil, err
	}
	if _, err := events.Append(dir, "session.created", map[string]any{
		"session_id":     metadata.SessionID,
		"session_type":   metadata.SessionType,
		"audio_path":     metadata.AudioPath,
		"paste_mode":     cfg.PasteMode,
		"selected_model": cfg.SelectedModel,
	}); err != nil {
		return nil, err
	}
	return metadata, nil
}

func SaveMetadata(metadata *Metadata) error {
	if metadata == nil || metadata.AudioPath == "" {
		return errors.New("session metadata has no audio path")
	}
	path := MetadataPath(SessionDir(metadata))
	return files.AtomicWriteJSON(path, metadata)
}

// UpdateMetadata persists fields already changed on metadata. Keeping the
// mutation explicit in Go makes it harder for later callers to update a field
// accidentally through a stringly-typed map.
func UpdateMetadata(metadata *Metadata) error { return SaveMetadata(metadata) }

func LoadSession(path string) (*Metadata, error) {
	metadataPath := MetadataPath(path)
	data, err := os.ReadFile(metadataPath)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var metadata Metadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		// A damaged directory should not make history listing fail. It remains
		// on disk for diagnosis, just as the Python reader leaves it alone.
		return nil, nil
	}
	return &metadata, nil
}

func All(cfg config.Config) ([]*Metadata, error) {
	entries, err := os.ReadDir(cfg.SessionsDir)
	if os.IsNotExist(err) {
		return []*Metadata{}, nil
	}
	if err != nil {
		return nil, err
	}
	var sessions []*Metadata
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		metadata, err := LoadSession(filepath.Join(cfg.SessionsDir, entry.Name()))
		if err != nil || metadata == nil {
			continue
		}
		sessions = append(sessions, metadata)
	}
	sort.Slice(sessions, func(i, j int) bool { return sessions[i].StartedAt > sessions[j].StartedAt })
	return sessions, nil
}

func Last(cfg config.Config) (*Metadata, error) {
	sessions, err := All(cfg)
	if err != nil || len(sessions) == 0 {
		return nil, err
	}
	return sessions[0], nil
}

func Find(cfg config.Config, id string) (*Metadata, error) {
	if id == "last" {
		return Last(cfg)
	}
	sessions, err := All(cfg)
	if err != nil {
		return nil, err
	}
	for _, metadata := range sessions {
		if metadata.SessionID == id {
			return metadata, nil
		}
	}
	return nil, nil
}

func MissingAudioMessage(metadata *Metadata) string {
	if metadata.AudioPrunedAt != nil {
		return fmt.Sprintf("Audio pruned at %s under audio_retention: %s", *metadata.AudioPrunedAt, metadata.AudioPath)
	}
	return "Audio missing: " + metadata.AudioPath
}

func PruneExpiredAudio(cfg config.Config) (int, error) {
	return PruneExpiredAudioAt(cfg, time.Now())
}

func PruneExpiredAudioAt(cfg config.Config, now time.Time) (int, error) {
	if cfg.AudioRetentionSeconds == nil {
		return 0, nil
	}
	sessions, err := All(cfg)
	if err != nil {
		return 0, err
	}
	cutoff := now.Add(-time.Duration(*cfg.AudioRetentionSeconds * float64(time.Second)))
	pruned := 0
	for _, metadata := range sessions {
		paths := []string{metadata.AudioPath}
		for _, path := range metadata.AudioSourcePaths {
			paths = append(paths, path)
		}
		expired := false
		for _, path := range paths {
			info, err := os.Stat(path)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return pruned, err
			}
			if info.ModTime().Before(cutoff) {
				expired = true
				break
			}
		}
		if !expired {
			continue
		}
		for _, path := range paths {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return pruned, err
			}
		}
		prunedAt := nowISO(now)
		metadata.AudioPrunedAt = &prunedAt
		if err := SaveMetadata(metadata); err != nil {
			return pruned, err
		}
		if _, err := events.Append(SessionDir(metadata), "audio.pruned", map[string]any{
			"audio_path":        metadata.AudioPath,
			"retention_seconds": *cfg.AudioRetentionSeconds,
		}); err != nil {
			return pruned, err
		}
		pruned++
	}
	return pruned, nil
}

func MarkIncompleteRecordingsRecovered(cfg config.Config) (int, error) {
	return MarkIncompleteRecordingsRecoveredAt(cfg, time.Now())
}

func MarkIncompleteRecordingsRecoveredAt(cfg config.Config, now time.Time) (int, error) {
	sessions, err := All(cfg)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, metadata := range sessions {
		if metadata.Status != "recording" {
			continue
		}
		metadata.Errors = append(metadata.Errors, recoveryMessage)
		metadata.Status = "recovered"
		if metadata.EndedAt == nil {
			ended := nowISO(now)
			metadata.EndedAt = &ended
		}
		if err := SaveMetadata(metadata); err != nil {
			return count, err
		}
		if _, err := events.Append(SessionDir(metadata), "session.recovered", map[string]any{
			"status": "recovered",
			"reason": "incomplete recording found on startup",
		}); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func RecoveryMessage() string { return recoveryMessage }
