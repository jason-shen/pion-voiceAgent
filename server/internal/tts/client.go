package tts

import (
	"context"
	"fmt"

	"github.com/voiceagent/server/internal/config"
)

// Client synthesizes text to PCM audio (linear16, 48kHz).
type Client interface {
	Synthesize(ctx context.Context, text string) ([]byte, error)
}

// NewClient returns a TTS client based on TTS_PROVIDER env var.
// Supports: "cartesia" (default), "deepgram"
func NewClient(cfg *config.Config) (Client, error) {
	provider := cfg.TTSProvider
	if provider == "" {
		provider = "cartesia"
	}

	switch provider {
	case "deepgram":
		if cfg.DeepgramAPIKey == "" {
			return nil, ErrMissingAPIKey{Provider: "deepgram", EnvVar: "DEEPGRAM_API_KEY"}
		}
		return NewDeepgramClient(cfg.DeepgramAPIKey), nil
	case "cartesia":
		if cfg.CartesiaAPIKey == "" {
			return nil, ErrMissingAPIKey{Provider: "cartesia", EnvVar: "CARTESIA_API_KEY"}
		}
		return NewCartesiaClient(cfg.CartesiaAPIKey, cfg.CartesiaVoiceID), nil
	default:
		return nil, fmt.Errorf("TTS_PROVIDER must be 'cartesia' or 'deepgram', got %q", provider)
	}
}
