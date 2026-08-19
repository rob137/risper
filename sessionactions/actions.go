// Package sessionactions contains the small actions shared by history and
// open. Keeping them here means both commands use the same legacy metadata
// fallbacks and desktop error messages.
package sessionactions

import (
	"os"
	"strings"

	"github.com/rob137/risper/config"
	"github.com/rob137/risper/desktop"
	"github.com/rob137/risper/session"
)

func TranscriptPath(metadata *session.Metadata) string {
	if metadata == nil {
		return ""
	}
	for _, path := range []string{metadata.TranscriptCleanPath, metadata.TranscriptRawPath} {
		if path == "" {
			continue
		}
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

func TranscriptPreview(metadata *session.Metadata, limit int) string {
	path := TranscriptPath(metadata)
	if path != "" {
		data, err := os.ReadFile(path)
		if err == nil {
			text := strings.Join(strings.Fields(string(data)), " ")
			return truncateRunes(text, limit)
		}
	}
	if metadata != nil && len(metadata.Errors) > 0 {
		return truncateRunes(metadata.Errors[len(metadata.Errors)-1], limit)
	}
	return ""
}

func OpenSession(metadata *session.Metadata) (bool, string) {
	if metadata == nil {
		return false, "session metadata is missing"
	}
	return desktop.OpenPath(session.SessionDir(metadata))
}

func PlayAudio(metadata *session.Metadata) (bool, string) {
	if metadata == nil {
		return false, "session metadata is missing"
	}
	if _, err := os.Stat(metadata.AudioPath); err != nil {
		return false, session.MissingAudioMessage(metadata)
	}
	return desktop.OpenPath(metadata.AudioPath)
}

func CopyTranscript(metadata *session.Metadata) (bool, string) {
	path := TranscriptPath(metadata)
	if path == "" {
		return false, "Session has no transcript."
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err.Error()
	}
	return desktop.CopyText(string(data))
}

func FindSessionOrError(cfg config.Config, sessionID string) (*session.Metadata, string, error) {
	metadata, err := session.Find(cfg, sessionID)
	if err != nil {
		return nil, "", err
	}
	if metadata == nil {
		return nil, "No such session: " + sessionID, nil
	}
	return metadata, "", nil
}

func truncateRunes(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit])
}
