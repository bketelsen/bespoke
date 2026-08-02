// The audio service (ADR-0014): transcription on the internal listener,
// Lemonade-backed when BESPOKE_LEMONADE_URL is set, stub mode otherwise
// (models not yet installed on selfie — roadmap backlog).
package main

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type audioGateway struct {
	base  string // Lemonade OpenAI-compatible base URL, e.g. http://selfie:8000/api/v1; empty = stub mode
	model string
	hc    *http.Client

	mu     sync.RWMutex
	status string
}

func newAudioGateway() *audioGateway {
	g := &audioGateway{
		base:  strings.TrimSuffix(os.Getenv("BESPOKE_LEMONADE_URL"), "/"),
		model: cmp.Or(os.Getenv("BESPOKE_AUDIO_MODEL"), "Whisper-Large-v3-Turbo"),
		hc:    &http.Client{Timeout: 120 * time.Second},
	}
	if !g.stub() {
		check := func() {
			resp, err := g.hc.Get(g.base + "/models")
			switch {
			case err != nil:
				g.setStatus(fmt.Sprintf("Audio: Lemonade unreachable at %s: %v", g.base, err))
			case resp.StatusCode != http.StatusOK:
				resp.Body.Close()
				g.setStatus(fmt.Sprintf("Audio: Lemonade returned %s", resp.Status))
			default:
				resp.Body.Close()
				g.setStatus("")
			}
		}
		check()
		go func() {
			for range time.Tick(5 * time.Minute) {
				check()
			}
		}()
	}
	return g
}

func (g *audioGateway) stub() bool { return g.base == "" }

func (g *audioGateway) setStatus(s string) {
	g.mu.Lock()
	g.status = s
	g.mu.Unlock()
	if s != "" {
		log.Println("audio:", s)
	}
}

func (g *audioGateway) warning() string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.status
}

func (g *audioGateway) transcribe(mime string, body io.Reader) (string, error) {
	if g.stub() {
		n, _ := io.Copy(io.Discard, body)
		return fmt.Sprintf("[voice entry — %dKB of audio received; transcription stubbed until Lemonade models are installed]",
			max(n/1024, 1)), nil
	}

	// OpenAI-compatible transcription request, validated against Lemonade
	// 2026-08-01: multipart file+model to /audio/transcriptions, WAV input
	// only (the pkg/ui recorder encodes WAV client-side), {text} response.
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", "audio"+extFor(mime))
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(fw, body); err != nil {
		return "", err
	}
	mw.WriteField("model", g.model)
	mw.Close()

	resp, err := g.hc.Post(g.base+"/audio/transcriptions", mw.FormDataContentType(), &buf)
	if err != nil {
		return "", fmt.Errorf("lemonade: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("lemonade: %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}
	var out struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.Text, nil
}

func extFor(mime string) string {
	switch {
	case strings.Contains(mime, "ogg"):
		return ".ogg"
	case strings.Contains(mime, "wav"):
		return ".wav"
	case strings.Contains(mime, "mp4"):
		return ".m4a"
	default:
		return ".webm"
	}
}

func (g *audioGateway) register(mux *http.ServeMux) {
	mux.HandleFunc("GET /audio/healthz", func(w http.ResponseWriter, r *http.Request) {
		if g.stub() {
			w.Write([]byte("ok (stub mode — BESPOKE_LEMONADE_URL unset)"))
			return
		}
		if s := g.warning(); s != "" {
			http.Error(w, s, http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte("ok"))
	})
	mux.HandleFunc("POST /audio/transcribe", func(w http.ResponseWriter, r *http.Request) {
		app := cmp.Or(r.Header.Get("X-Bespoke-App"), "unknown")
		start := time.Now()
		text, err := g.transcribe(r.Header.Get("Content-Type"), http.MaxBytesReader(w, r.Body, 25<<20))
		mode := "lemonade"
		if g.stub() {
			mode = "stub"
		}
		log.Printf("audio app=%s mode=%s out=%dB dur=%s err=%v",
			app, mode, len(text), time.Since(start).Round(time.Millisecond), err)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"text": text})
	})
}
