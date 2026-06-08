// Package voice implements the bismuth voice gateway.
//
// V1 approach (streaming HTTP, no LiveKit):
//
//   1. Browser PWA captures audio via MediaRecorder (16kHz PCM WebM).
//   2. POST chunked to /v1/voice/stt on bismuth.
//   3. bismuth forwards audio to 9router /v1/audio/transcriptions
//      (provider: groq whisper-large-v3-turbo or deepgram nova-3).
//   4. Resulting text is fed to a small command parser that maps to
//      bismuth API calls (bismuth.spawn / bismuth.send / bismuth.status
//      / bismuth.kill / bismuth.merge / open_app / change_role / ...).
//   5. Response text is sent back to TTS via 9router /v1/audio/speech
//      (provider: edge-tts or elevenlabs), streamed to the browser.
//
// V2 will swap the HTTP path for a LiveKit room (real turn-taking,
// interruption, VAD proper, barge-in).
package voice

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/biodoia/bismuth/internal/bus"
	"github.com/biodoia/bismuth/internal/config"
)

// Gateway exposes STT + TTS + command-parse + speak.
type Gateway struct {
	cfg config.VoiceCfg
	db  *sql.DB
	bus *bus.Bus
	hc  *http.Client

	nineRURL string
	nineRKey string
}

// NewGateway creates the voice gateway.
func NewGateway(ctx context.Context, cfg config.VoiceCfg, db *sql.DB, b *bus.Bus) (*Gateway, error) {
	url := osGetenv("NINEROUTER_URL", "")
	key := osGetenv("NINEROUTER_KEY", "")
	if url == "" {
		return nil, fmt.Errorf("NINEROUTER_URL must be set")
	}
	return &Gateway{
		cfg:      cfg,
		db:       db,
		bus:      b,
		hc:       &http.Client{Timeout: 30 * time.Second},
		nineRURL: url,
		nineRKey: key,
	}, nil
}

// Close releases resources.
func (g *Gateway) Close() error { return nil }

// Transcribe sends audio bytes to 9router STT and returns text.
//
// audio: typically webm/opus from MediaRecorder, or wav/pcm.
// lang: ISO-639-1 ("it", "en", ...).
func (g *Gateway) Transcribe(ctx context.Context, audio []byte, lang string) (string, error) {
	if lang == "" {
		lang = g.cfg.Language
	}
	model := g.cfg.STTModel
	if model == "" {
		model = "whisper-large-v3-turbo"
	}
	url := g.nineRURL + "/v1/audio/transcriptions"

	body := &bytes.Buffer{}
	w := multipartWriter(body)
	_ = w.WriteField("model", model)
	_ = w.WriteField("language", lang)
	_ = w.WriteFile("file", "audio.webm", audio)
	_ = w.Close()

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	req.Header.Set("Content-Type", w.ContentType())
	if g.nineRKey != "" {
		req.Header.Set("Authorization", "Bearer "+g.nineRKey)
	}

	resp, err := g.hc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("9router STT %d: %s", resp.StatusCode, string(b))
	}
	var out struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return strings.TrimSpace(out.Text), nil
}

// Speak synthesizes text to audio and returns the bytes (mp3).
func (g *Gateway) Speak(ctx context.Context, text string) ([]byte, error) {
	url := g.nineRURL + "/v1/audio/speech"
	body, _ := json.Marshal(map[string]any{
		"model": g.cfg.TTSVoice,
		"input": text,
	})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if g.nineRKey != "" {
		req.Header.Set("Authorization", "Bearer "+g.nineRKey)
	}
	resp, err := g.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("9router TTS %d: %s", resp.StatusCode, string(b))
	}
	return io.ReadAll(resp.Body)
}

// SpeakBase64 is the same as Speak but returns base64 for the JSON API.
func (g *Gateway) SpeakBase64(ctx context.Context, text string) (string, string, error) {
	b, err := g.Speak(ctx, text)
	if err != nil {
		return "", "", err
	}
	return base64.StdEncoding.EncodeToString(b), "mp3", nil
}
