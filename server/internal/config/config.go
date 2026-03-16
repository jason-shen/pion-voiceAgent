package config

import (
	"os"
)

type Config struct {
	Port           string
	DeepgramAPIKey string
	OpenAIAPIKey   string
	SystemPrompt   string

	// TTS provider: "cartesia" (default) or "deepgram"
	TTSProvider    string
	CartesiaAPIKey string
	CartesiaVoiceID string
}

func Load() *Config {
	return &Config{
		Port:            getEnv("PORT", "8080"),
		DeepgramAPIKey:  os.Getenv("DEEPGRAM_API_KEY"),
		OpenAIAPIKey:    os.Getenv("OPENAI_API_KEY"),
		SystemPrompt:    getEnv("SYSTEM_PROMPT", "You are a helpful AI voice assistant. Keep your responses concise and conversational."),
		TTSProvider:     getEnv("TTS_PROVIDER", "cartesia"),
		CartesiaAPIKey:  os.Getenv("CARTESIA_API_KEY"),
		CartesiaVoiceID: getEnv("CARTESIA_VOICE_ID", ""),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
