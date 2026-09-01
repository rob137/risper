package transcription

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rob137/risper/models"
)

const (
	defaultOpenAIEndpoint = "https://api.openai.com/v1/audio/transcriptions"
	defaultOpenAIKeyFile  = "~/.config/openai/key"
	maxOpenAIKeyBytes     = 64 * 1024
	maxOpenAIResponseSize = 1 * 1024 * 1024
)

var (
	openAIEndpoint       = defaultOpenAIEndpoint
	openAIRequestTimeout = 2 * time.Minute
)

// openAIInputError marks a failure reading the recording itself. A fallback
// engine cannot make an unavailable recording available, so callers should
// report this error without starting another transcription process.
type openAIInputError struct {
	err error
}

func (err *openAIInputError) Error() string { return err.err.Error() }
func (err *openAIInputError) Unwrap() error { return err.err }

// transcriptWriteError marks a failure after a transcript has been received.
// Retrying with another engine could overwrite useful diagnostics or hide a
// storage problem, so this error is deliberately not fallback-eligible.
type transcriptWriteError struct {
	err error
}

func (err *transcriptWriteError) Error() string { return err.err.Error() }
func (err *transcriptWriteError) Unwrap() error { return err.err }

// transcribeOpenAI streams the audio into the multipart request. The API key
// is read only into memory and is never put in a profile command, URL, or
// child-process environment.
func transcribeOpenAI(profile models.Profile, audioPath, rawPath, cleanPath string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), openAIRequestTimeout)
	defer cancel()
	return transcribeOpenAIContext(ctx, profile, audioPath, rawPath, cleanPath)
}

func transcribeOpenAIContext(ctx context.Context, profile models.Profile, audioPath, rawPath, cleanPath string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	key, err := readOpenAIKey(profile.APIKeyFile)
	if err != nil {
		return "", err
	}
	audio, err := os.Open(audioPath)
	if err != nil {
		return "", &openAIInputError{err: fmt.Errorf("open audio for OpenAI transcription: %w", err)}
	}

	reader, writer := io.Pipe()
	form := multipart.NewWriter(writer)
	multipartDone := make(chan error, 1)
	go func() {
		multipartDone <- writeOpenAIMultipart(form, writer, audio, audioPath, profile)
	}()
	stopMultipart := func(cause error) error {
		if cause != nil {
			_ = reader.CloseWithError(cause)
		} else {
			_ = reader.Close()
		}
		return <-multipartDone
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, openAIEndpoint, reader)
	if err != nil {
		multipartErr := stopMultipart(err)
		var inputErr *openAIInputError
		if errors.As(multipartErr, &inputErr) {
			return "", inputErr
		}
		return "", fmt.Errorf("create OpenAI transcription request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+key)
	request.Header.Set("Content-Type", form.FormDataContentType())

	response, err := (&http.Client{}).Do(request)
	if err != nil {
		// A timeout or transport failure can happen while the multipart writer
		// is blocked on the pipe. Closing the reader releases that goroutine.
		multipartErr := stopMultipart(err)
		var inputErr *openAIInputError
		if errors.As(multipartErr, &inputErr) {
			return "", inputErr
		}
		return "", fmt.Errorf("OpenAI transcription request failed: %w", err)
	}
	defer response.Body.Close()
	body, err := readOpenAIBody(response.Body)
	multipartErr := stopMultipart(nil)
	var inputErr *openAIInputError
	if errors.As(multipartErr, &inputErr) {
		return "", inputErr
	}
	if err != nil {
		return "", err
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return "", openAIHTTPError(response.StatusCode, body, key)
	}

	var result struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("OpenAI transcription returned invalid JSON: %w", err)
	}
	text := strings.TrimSpace(result.Text)
	if text == "" {
		return "", ErrNoTranscript{}
	}
	if err := writeTranscript(rawPath, cleanPath, text); err != nil {
		return "", &transcriptWriteError{err: err}
	}
	return text, nil
}

func writeOpenAIMultipart(form *multipart.Writer, pipe *io.PipeWriter, audio *os.File, audioPath string, profile models.Profile) error {
	defer audio.Close()
	part, err := form.CreateFormFile("file", filepath.Base(audioPath))
	if err == nil {
		if _, copyErr := io.Copy(part, audio); copyErr != nil {
			if errors.Is(copyErr, io.ErrClosedPipe) {
				err = copyErr
			} else {
				err = &openAIInputError{err: fmt.Errorf("read audio for OpenAI transcription: %w", copyErr)}
			}
		}
	}
	if err == nil {
		err = form.WriteField("model", profile.Model)
	}
	if err == nil && profile.Language != "" {
		err = form.WriteField("language", profile.Language)
	}
	if err == nil && profile.Prompt != "" {
		err = form.WriteField("prompt", profile.Prompt)
	}
	if err == nil {
		err = form.WriteField("response_format", "json")
	}
	if closeErr := form.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = pipe.CloseWithError(err)
		return err
	}
	return pipe.Close()
}

func readOpenAIKey(path string) (string, error) {
	path = expandOpenAIPath(path)
	if path == "" {
		return "", errors.New("OpenAI API key file path is empty")
	}
	handle, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open OpenAI API key file %s: %w", path, err)
	}
	defer handle.Close()
	info, err := handle.Stat()
	if err != nil {
		return "", fmt.Errorf("stat OpenAI API key file %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("OpenAI API key file %s is not a regular file", path)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return "", fmt.Errorf("OpenAI API key file %s must have no group/world permissions (use mode 0600 or 0400)", path)
	}
	data, err := io.ReadAll(io.LimitReader(handle, maxOpenAIKeyBytes+1))
	if err != nil {
		return "", fmt.Errorf("read OpenAI API key file %s: %w", path, err)
	}
	if len(data) > maxOpenAIKeyBytes {
		return "", fmt.Errorf("OpenAI API key file %s is too large", path)
	}
	key := strings.TrimSpace(string(data))
	if key == "" {
		return "", fmt.Errorf("OpenAI API key file %s is empty", path)
	}
	return key, nil
}

func expandOpenAIPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		path = defaultOpenAIKeyFile
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		if path == "~" {
			return home
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

func readOpenAIBody(body io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(body, maxOpenAIResponseSize+1))
	if err != nil {
		return nil, fmt.Errorf("read OpenAI transcription response: %w", err)
	}
	if len(data) > maxOpenAIResponseSize {
		return nil, fmt.Errorf("OpenAI transcription response exceeds %d-byte limit", maxOpenAIResponseSize)
	}
	return data, nil
}

func openAIHTTPError(status int, body []byte, key string) error {
	var payload struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	message := ""
	if json.Unmarshal(body, &payload) == nil {
		message = payload.Error.Message
	}
	if message == "" {
		message = strings.TrimSpace(string(body))
	}
	message = sanitizeOpenAIMessage(message, key)
	message = strings.Join(strings.Fields(message), " ")
	if message == "" {
		return fmt.Errorf("OpenAI transcription failed with HTTP status %d", status)
	}
	if runes := []rune(message); len(runes) > 512 {
		message = string(runes[:512]) + "..."
	}
	return fmt.Errorf("OpenAI transcription failed with HTTP status %d: %s", status, message)
}

func sanitizeOpenAIMessage(message, key string) string {
	if key != "" {
		message = strings.ReplaceAll(message, key, "[REDACTED]")
	}
	return message
}

// Codex gpt-5.6-sol, xhigh, prompted by Robert Kirby
