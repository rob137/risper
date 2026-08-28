package transcription

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rob137/risper/models"
)

func openAITestKey(t *testing.T, contents string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "openai-key")
	if err := os.WriteFile(path, []byte(contents), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
	return path
}

func useOpenAITestServer(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	oldEndpoint := openAIEndpoint
	openAIEndpoint = server.URL
	t.Cleanup(func() { openAIEndpoint = oldEndpoint })
}

func useOpenAITestTimeout(t *testing.T, timeout time.Duration) {
	t.Helper()
	oldTimeout := openAIRequestTimeout
	openAIRequestTimeout = timeout
	t.Cleanup(func() { openAIRequestTimeout = oldTimeout })
}

func TestTranscribeOpenAIStreamsAudioAndPreservesFields(t *testing.T) {
	const key = "sk-test-stream-key"
	const prompt = `Names: Rob's "catchphrase", coEngen, Δ tok.`
	audioData := []byte("RIFF test audio bytes")
	keyPath := openAITestKey(t, "\n"+key+"\n", 0o600)
	root := t.TempDir()
	rawPath := filepath.Join(root, "raw.txt")
	cleanPath := filepath.Join(root, "clean.txt")
	audioPath := filepath.Join(root, "clip.wav")
	if err := os.WriteFile(audioPath, audioData, 0o644); err != nil {
		t.Fatal(err)
	}

	useOpenAITestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+key {
			t.Errorf("authorization = %q", got)
		}
		if strings.Contains(r.URL.String(), key) {
			t.Errorf("key appeared in request URL: %q", r.URL.String())
		}
		if err := r.ParseMultipartForm(64 * 1024); err != nil {
			t.Fatalf("parse multipart form: %v", err)
		}
		for field, want := range map[string]string{
			"model":           "gpt-transcribe",
			"language":        "en",
			"prompt":          prompt,
			"response_format": "json",
		} {
			if got := r.FormValue(field); got != want {
				t.Errorf("%s = %q, want %q", field, got, want)
			}
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("file field: %v", err)
		}
		defer file.Close()
		if header.Filename != "clip.wav" {
			t.Errorf("filename = %q, want clip.wav", header.Filename)
		}
		gotAudio, err := io.ReadAll(file)
		if err != nil {
			t.Fatalf("read uploaded audio: %v", err)
		}
		if string(gotAudio) != string(audioData) {
			t.Errorf("uploaded audio = %q, want %q", gotAudio, audioData)
		}
		body, err := io.ReadAll(r.Body)
		if err == nil && strings.Contains(string(body), key) {
			t.Error("API key appeared in multipart body")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"text":"cloud transcript"}`)
	})

	transcript, err := Transcribe(models.Profile{
		Engine: "openai", Model: "gpt-transcribe", Language: "en", Prompt: prompt, APIKeyFile: keyPath,
	}, audioPath, rawPath, cleanPath, func(pid int) error {
		return errors.New("native engine unexpectedly started a child process")
	})
	if err != nil || transcript != "cloud transcript" {
		t.Fatalf("transcript = %q, err = %v", transcript, err)
	}
	for _, path := range []string{rawPath, cleanPath} {
		if got, err := os.ReadFile(path); err != nil || string(got) != "cloud transcript\n" {
			t.Errorf("%s = %q, %v", path, got, err)
		}
	}
}

func TestTranscribeOpenAIRejectsBadKeyFiles(t *testing.T) {
	tests := []struct {
		name string
		path func(t *testing.T) string
		want string
	}{
		{name: "missing", path: func(t *testing.T) string { return filepath.Join(t.TempDir(), "missing") }, want: "open OpenAI API key file"},
		{name: "empty", path: func(t *testing.T) string { return openAITestKey(t, " \n", 0o600) }, want: "is empty"},
		{name: "readable by group", path: func(t *testing.T) string { return openAITestKey(t, "key", 0o640) }, want: "must have no group/world permissions"},
		{name: "directory", path: func(t *testing.T) string {
			path := filepath.Join(t.TempDir(), "key-dir")
			if err := os.Mkdir(path, 0o700); err != nil {
				t.Fatal(err)
			}
			return path
		}, want: "is not a regular file"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := transcribeOpenAI(models.Profile{Engine: "openai", APIKeyFile: test.path(t)}, "missing.wav", "raw.txt", "clean.txt")
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestTranscribeOpenAIUsesDefaultKeyPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, ".config", "openai")
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "key"), []byte("default-key"), 0o600); err != nil {
		t.Fatal(err)
	}
	useOpenAITestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer default-key" {
			t.Errorf("authorization = %q", got)
		}
		_, _ = io.WriteString(w, `{"text":"default key transcript"}`)
	})
	audioPath := filepath.Join(t.TempDir(), "clip.wav")
	if err := os.WriteFile(audioPath, []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	transcript, err := transcribeOpenAI(models.Profile{Engine: "openai", Model: "gpt-transcribe", Language: "en"}, audioPath, filepath.Join(t.TempDir(), "raw"), filepath.Join(t.TempDir(), "clean"))
	if err != nil || transcript != "default key transcript" {
		t.Fatalf("transcript = %q, err = %v", transcript, err)
	}
}

func TestTranscribeOpenAISafelyReportsHTTPError(t *testing.T) {
	const key = "sk-never-log-this"
	keyPath := openAITestKey(t, key, 0o600)
	audioPath := filepath.Join(t.TempDir(), "clip.wav")
	if err := os.WriteFile(audioPath, []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	useOpenAITestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"message":"bad key sk-never-log-this"}}`)
	})
	_, err := transcribeOpenAI(models.Profile{Engine: "openai", APIKeyFile: keyPath}, audioPath, "raw.txt", "clean.txt")
	if err == nil || !strings.Contains(err.Error(), "HTTP status 401") {
		t.Fatalf("error = %v, want HTTP 401", err)
	}
	if strings.Contains(err.Error(), key) {
		t.Fatalf("error leaked API key: %v", err)
	}
}

func TestTranscribeOpenAIRejectsInvalidAndOversizedJSON(t *testing.T) {
	keyPath := openAITestKey(t, "test-key", 0o600)
	root := t.TempDir()
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "malformed", body: `{"text":`, want: "invalid JSON"},
		{name: "missing text", body: `{"language":"en"}`, want: "no transcript"},
		{name: "empty text", body: `{"text":"  "}`, want: "no transcript"},
		{name: "trailing object", body: `{"text":"ok"}{"text":"bad"}`, want: "invalid JSON"},
		{name: "wrong text type", body: `{"text":42}`, want: "invalid JSON"},
		{name: "oversized", body: `{"text":"` + strings.Repeat("x", maxOpenAIResponseSize) + `"}`, want: "exceeds"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			useOpenAITestServer(t, func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.WriteString(w, test.body)
			})
			rawPath := filepath.Join(root, test.name+".raw")
			cleanPath := filepath.Join(root, test.name+".clean")
			audioPath := filepath.Join(root, test.name+".wav")
			if err := os.WriteFile(audioPath, []byte("audio"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(rawPath, []byte("stale raw\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(cleanPath, []byte("stale clean\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := transcribeOpenAI(models.Profile{Engine: "openai", Model: "gpt-transcribe", APIKeyFile: keyPath}, audioPath, rawPath, cleanPath)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
			for _, path := range []string{rawPath, cleanPath} {
				got, readErr := os.ReadFile(path)
				want := "stale raw\n"
				if strings.HasSuffix(path, ".clean") {
					want = "stale clean\n"
				}
				if readErr != nil || string(got) != want {
					t.Errorf("%s changed to %q, want %q (%v)", path, got, want, readErr)
				}
			}
		})
	}
}

func TestTranscribeOpenAITimesOut(t *testing.T) {
	keyPath := openAITestKey(t, "test-key", 0o600)
	audioPath := filepath.Join(t.TempDir(), "clip.wav")
	if err := os.WriteFile(audioPath, []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	useOpenAITestTimeout(t, 40*time.Millisecond)
	useOpenAITestServer(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
	})
	started := time.Now()
	_, err := transcribeOpenAI(models.Profile{Engine: "openai", Model: "gpt-transcribe", APIKeyFile: keyPath}, audioPath, "raw.txt", "clean.txt")
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want deadline exceeded", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("timeout took %s", elapsed)
	}
}

func TestOpenAIErrorPayloadExtraction(t *testing.T) {
	body := []byte(`{"error":{"message":"rate limited"}}`)
	err := openAIHTTPError(http.StatusTooManyRequests, body, "key")
	if err == nil || !strings.Contains(err.Error(), "HTTP status 429") || !strings.Contains(err.Error(), "rate limited") {
		t.Fatalf("error = %v", err)
	}
	var payload struct {
		Error map[string]string `json:"error"`
	}
	if json.Unmarshal(body, &payload) != nil {
		t.Fatal("test payload is not JSON")
	}
}

// Codex gpt-5.6-sol, xhigh, prompted by Robert Kirby
