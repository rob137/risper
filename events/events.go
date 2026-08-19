package events

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Record map[string]any

const FileName = "events.jsonl"

func Path(sessionDir string) string { return filepath.Join(sessionDir, FileName) }

func Append(sessionDir, event string, fields map[string]any) (Record, error) {
	return AppendAt(sessionDir, event, time.Now(), fields)
}

func AppendAt(sessionDir, event string, timestamp time.Time, fields map[string]any) (Record, error) {
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		return nil, err
	}
	record := Record{"timestamp": timestamp.Format(time.RFC3339), "event": event}
	for key, value := range fields {
		record[key] = value
	}
	data, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("marshal event: %w", err)
	}
	handle, err := os.OpenFile(Path(sessionDir), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	if _, writeErr := handle.Write(append(data, '\n')); writeErr != nil {
		handle.Close()
		return nil, writeErr
	}
	if err := handle.Close(); err != nil {
		return nil, err
	}
	return record, nil
}

func Read(sessionDir string, limit int) ([]Record, error) {
	handle, err := os.Open(Path(sessionDir))
	if os.IsNotExist(err) {
		return []Record{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer handle.Close()

	var records []Record
	scanner := bufio.NewScanner(handle)
	scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		var record Record
		if err := json.Unmarshal(line, &record); err != nil || record == nil {
			record = Record{"event": "diagnostic.invalid_event_line", "raw": scanner.Text()}
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if limit > 0 && len(records) > limit {
		return records[len(records)-limit:], nil
	}
	return records, nil
}
